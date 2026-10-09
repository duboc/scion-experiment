package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// env returns a getenv func backed by a map, so tests never read the real
// environment.
func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestParseConfigListenAddr(t *testing.T) {
	tests := []struct {
		name string
		args []string
		port string // "" means PORT unset or empty
		want string
	}{
		{"default", nil, "", ":8080"},
		{"PORT used when -addr not set", nil, "9090", ":9090"},
		{"PORT lower bound", nil, "1", ":1"},
		{"PORT upper bound", nil, "65535", ":65535"},
		{"PORT leading zeros normalised", nil, "0080", ":80"},
		{"explicit -addr beats PORT", []string{"-addr", ":7000"}, "9090", ":7000"},
		{"explicit -addr equal to default still beats PORT", []string{"-addr", ":8080"}, "9090", ":8080"},
		{"explicit -addr=value form beats PORT", []string{"-addr=127.0.0.1:7001"}, "9090", "127.0.0.1:7001"},
		{"explicit -addr ignores invalid PORT", []string{"-addr", ":7000"}, "not-a-port", ":7000"},
		{"other flags do not count as -addr", []string{"-static", "/srv/ui"}, "9090", ":9090"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig(tc.args, env(map[string]string{"PORT": tc.port}))
			if err != nil {
				t.Fatalf("parseConfig(%q) with PORT=%q error: %v", tc.args, tc.port, err)
			}
			if cfg.addr != tc.want {
				t.Errorf("addr = %q, want %q", cfg.addr, tc.want)
			}
		})
	}
}

func TestParseConfigInvalidPORT(t *testing.T) {
	for _, port := range []string{"abc", "0", "65536", "-1", "+80", " 80", "80 ", "80.0", "8080a", "0x50", "99999999999999999999"} {
		t.Run(strconv.Quote(port), func(t *testing.T) {
			_, err := parseConfig(nil, env(map[string]string{"PORT": port}))
			if err == nil {
				t.Fatalf("parseConfig with PORT=%q = nil error, want error", port)
			}
			if want := "invalid PORT " + strconv.Quote(port); !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to contain %q", err, want)
			}
		})
	}
}

func TestParseConfigStaticDir(t *testing.T) {
	cfg, err := parseConfig(nil, env(nil))
	if err != nil || cfg.staticDir != "../frontend" {
		t.Errorf("default static = %q, %v; want ../frontend", cfg.staticDir, err)
	}
	cfg, err = parseConfig([]string{"-static", "/app/frontend"}, env(nil))
	if err != nil || cfg.staticDir != "/app/frontend" {
		t.Errorf("static = %q, %v; want /app/frontend", cfg.staticDir, err)
	}
}

func TestParseConfigRejectsBadArgs(t *testing.T) {
	tests := map[string][]string{
		"unknown flag":   {"-port", "1"},
		"extra argument": {"-addr", "127.0.0.1:0", "extra"},
		"missing value":  {"-addr"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(args, env(nil)); err == nil {
				t.Errorf("parseConfig(%q) = nil error, want error", args)
			}
		})
	}
}

// startRun runs the server in the background and returns its bound address
// and a stop function that cancels it and returns run's error.
func startRun(t *testing.T, args []string, getenv func(string) string) (addr string, stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- run(ctx, args, getenv, discardLogger(), ready) }()

	select {
	case addr = <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("run exited early: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("server did not become ready")
	}
	return addr, func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("server did not shut down")
			return nil
		}
	}
}

func httpGet(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestRunServesAndShutsDownGracefully(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>ui</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// PORT is set but must be ignored because -addr is explicit.
	addr, stop := startRun(t, []string{"-addr", "127.0.0.1:0", "-static", dir}, env(map[string]string{"PORT": "1"}))

	for path, want := range map[string]string{"/healthz": `{"status":"ok"}`, "/": "<h1>ui</h1>"} {
		code, body := httpGet(t, "http://"+addr+path)
		if code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("GET %s = %d %q, want 200 containing %q", path, code, body, want)
		}
	}
	if err := stop(); err != nil {
		t.Errorf("run returned %v after cancel, want nil", err)
	}
}

func TestRunListensOnPORT(t *testing.T) {
	// Reserve a free port, release it, and hand it to the server via PORT.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	addr, stop := startRun(t, []string{"-static", t.TempDir()}, env(map[string]string{"PORT": port}))
	if !strings.HasSuffix(addr, ":"+port) {
		t.Errorf("bound address = %q, want port %s", addr, port)
	}
	if code, _ := httpGet(t, "http://127.0.0.1:"+port+"/healthz"); code != http.StatusOK {
		t.Errorf("GET /healthz on PORT = %d, want 200", code)
	}
	if err := stop(); err != nil {
		t.Errorf("run returned %v after cancel, want nil", err)
	}
}

func TestRunRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name string
		args []string
		port string
	}{
		{"unknown flag", []string{"-port", "1"}, ""},
		{"bad address", []string{"-addr", "not-an-address"}, ""},
		{"invalid PORT", nil, "http"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(context.Background(), tc.args, env(map[string]string{"PORT": tc.port}), discardLogger(), nil); err == nil {
				t.Errorf("run(%q, PORT=%q) = nil, want error", tc.args, tc.port)
			}
		})
	}
}
