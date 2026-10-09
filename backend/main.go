// Command incidentdash serves the Incident Status Dashboard JSON API and the
// static UI. See /workspace/DESIGN.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"incidentdash/internal/api"
	"incidentdash/internal/store"
)

const (
	defaultAddr     = ":8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(ctx, os.Args[1:], os.Getenv, logger, nil); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

// config is the server configuration derived from flags and environment.
type config struct {
	addr      string // listen address, e.g. ":8080"
	staticDir string // directory served at /
}

// parseConfig parses command-line flags and the environment (via getenv).
//
// The listen address is chosen in this order:
//  1. -addr, if given explicitly (even if equal to the default);
//  2. ":"+$PORT, if PORT is non-empty (Cloud Run injects it);
//  3. the -addr default, ":8080".
//
// PORT must be a decimal number from 1 to 65535 when it is used.
func parseConfig(args []string, getenv func(string) string) (config, error) {
	fs := flag.NewFlagSet("incidentdash", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", defaultAddr, "listen address (overrides $PORT)")
	staticDir := fs.String("static", "../frontend", "directory served at /")
	if err := fs.Parse(args); err != nil {
		return config{}, fmt.Errorf("parsing flags: %w", err)
	}
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	cfg := config{addr: *addr, staticDir: *staticDir}
	addrSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrSet = true
		}
	})
	if port := getenv("PORT"); !addrSet && port != "" {
		// ParseUint rejects signs, spaces and non-digits; 16 bits caps at 65535.
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return config{}, fmt.Errorf("invalid PORT %q: must be a number from 1 to 65535", port)
		}
		cfg.addr = ":" + strconv.FormatUint(n, 10)
	}
	return cfg, nil
}

// run parses flags and environment, serves until ctx is cancelled, then
// shuts down gracefully. If ready is non-nil, it receives the bound address
// once the server is listening.
func run(ctx context.Context, args []string, getenv func(string) string, logger *slog.Logger, ready chan<- string) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}

	if info, err := os.Stat(cfg.staticDir); err != nil || !info.IsDir() {
		logger.Warn("static directory not found; UI will return 404", "static", cfg.staticDir)
	}

	st := store.NewSeeded(time.Now)
	srv := &http.Server{
		Handler:           api.New(st, cfg.staticDir, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", cfg.addr, err)
	}
	logger.Info("serving", "addr", ln.Addr().String(), "static", cfg.staticDir)
	if ready != nil {
		ready <- ln.Addr().String()
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
