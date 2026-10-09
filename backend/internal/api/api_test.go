package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"incidentdash/internal/store"
)

var fixedNow = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

const indexHTML = `<!doctype html><title>dash</title><div data-testid="overall-status"></div>`

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.NewSeeded(func() time.Time { return fixedNow })
	return New(st, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

type response struct {
	code   int
	header http.Header
	body   []byte
}

// do sends a request; POSTs carry Content-Type: application/json.
func do(t *testing.T, h http.Handler, method, target, body string) response {
	t.Helper()
	contentType := ""
	if method == http.MethodPost {
		contentType = "application/json"
	}
	return doWithContentType(t, h, method, target, contentType, body)
}

// doWithContentType sends a request with the given Content-Type header,
// omitting the header when contentType is empty.
func doWithContentType(t *testing.T, h http.Handler, method, target, contentType, body string) response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return response{code: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

func decode[T any](t *testing.T, res response) T {
	t.Helper()
	if ct := res.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var v T
	dec := json.NewDecoder(strings.NewReader(string(res.body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decoding %T from %s: %v", v, res.body, err)
	}
	return v
}

// expectError asserts that res is a well-formed error envelope.
func expectError(t *testing.T, res response, wantStatus int, wantCode, wantMsgSubstr string) {
	t.Helper()
	if res.code != wantStatus {
		t.Errorf("status = %d, want %d (body %s)", res.code, wantStatus, res.body)
	}
	env := decode[errorBody](t, res)
	if env.Error.Code != wantCode {
		t.Errorf("error.code = %q, want %q", env.Error.Code, wantCode)
	}
	if !strings.Contains(env.Error.Message, wantMsgSubstr) {
		t.Errorf("error.message = %q, want it to contain %q", env.Error.Message, wantMsgSubstr)
	}
}

// Wire types mirror DESIGN.md §6 independently of the store's Go types, so
// the tests catch accidental JSON field renames.
type wireService struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type wireSummary struct {
	OverallStatus string        `json:"overall_status"`
	OpenIncidents int           `json:"open_incidents"`
	Services      []wireService `json:"services"`
}

type wireUpdate struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type wireIncident struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	ServiceID   string       `json:"service_id"`
	Severity    string       `json:"severity"`
	Status      string       `json:"status"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	ResolvedAt  *string      `json:"resolved_at"`
	Updates     []wireUpdate `json:"updates"`
}

func TestHealthzHead(t *testing.T) {
	res := do(t, newTestHandler(t), http.MethodHead, "/healthz", "")
	if res.code != http.StatusOK {
		t.Errorf("HEAD /healthz status = %d, want 200", res.code)
	}
}

func TestHealthz(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodGet, "/healthz", "")
	if res.code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.code)
	}
	if got := decode[map[string]string](t, res); got["status"] != "ok" || len(got) != 1 {
		t.Errorf("body = %v, want {status: ok}", got)
	}
}

func TestSummary(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodGet, "/api/v1/summary", "")
	if res.code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.code)
	}
	got := decode[wireSummary](t, res)
	if got.OverallStatus != "partial_outage" || got.OpenIncidents != 1 || len(got.Services) != 5 {
		t.Errorf("summary = %+v, want partial_outage, 1 open, 5 services", got)
	}
}

func TestServices(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodGet, "/api/v1/services", "")
	if res.code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.code)
	}
	got := decode[struct {
		Services []wireService `json:"services"`
	}](t, res)
	want := []wireService{
		{"api", "Public API", "operational"},
		{"auth", "Authentication", "operational"},
		{"db", "Database", "operational"},
		{"payments", "Payments", "partial_outage"},
		{"web", "Web App", "operational"},
	}
	if len(got.Services) != len(want) {
		t.Fatalf("services = %+v, want %+v", got.Services, want)
	}
	for i := range want {
		if got.Services[i] != want[i] {
			t.Errorf("services[%d] = %+v, want %+v", i, got.Services[i], want[i])
		}
	}
}

func listIDs(t *testing.T, h http.Handler, target string) []string {
	t.Helper()
	res := do(t, h, http.MethodGet, target, "")
	if res.code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body %s", target, res.code, res.body)
	}
	got := decode[struct {
		Incidents []wireIncident `json:"incidents"`
	}](t, res)
	if got.Incidents == nil {
		t.Fatalf("GET %s: incidents is null, want array", target)
	}
	ids := make([]string, len(got.Incidents))
	for i, inc := range got.Incidents {
		ids[i] = inc.ID
	}
	return ids
}

func TestListIncidents(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		target string
		want   string
	}{
		{"/api/v1/incidents", "inc-2,inc-1"},
		{"/api/v1/incidents?status=open", "inc-2"},
		{"/api/v1/incidents?status=resolved", "inc-1"},
		{"/api/v1/incidents?service_id=web", "inc-1"},
		{"/api/v1/incidents?service_id=payments&status=resolved", ""},
		{"/api/v1/incidents?status=", "inc-2,inc-1"},
	}
	for _, tc := range tests {
		if got := strings.Join(listIDs(t, h, tc.target), ","); got != tc.want {
			t.Errorf("GET %s = [%s], want [%s]", tc.target, got, tc.want)
		}
	}
}

func TestListIncidentsInvalidFilters(t *testing.T) {
	h := newTestHandler(t)
	expectError(t, do(t, h, http.MethodGet, "/api/v1/incidents?status=closed", ""),
		http.StatusBadRequest, "invalid_argument", "status filter")
	expectError(t, do(t, h, http.MethodGet, "/api/v1/incidents?service_id=nope", ""),
		http.StatusBadRequest, "invalid_argument", `unknown service_id "nope"`)
}

func TestIncidentWireFormat(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodGet, "/api/v1/incidents/inc-1", "")
	if res.code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.code)
	}
	inc := decode[wireIncident](t, res)
	if inc.ID != "inc-1" || inc.ServiceID != "web" || inc.Severity != "sev3" || inc.Status != "resolved" {
		t.Errorf("incident = %+v", inc)
	}
	if inc.ResolvedAt == nil || *inc.ResolvedAt != "2026-10-02T10:00:00Z" {
		t.Errorf("resolved_at = %v, want 2026-10-02T10:00:00Z", inc.ResolvedAt)
	}
	for _, ts := range []string{inc.CreatedAt, inc.UpdatedAt, inc.Updates[0].CreatedAt} {
		if _, err := time.Parse(time.RFC3339, ts); err != nil || !strings.HasSuffix(ts, "Z") {
			t.Errorf("timestamp %q is not RFC3339 UTC", ts)
		}
	}

	// Open incidents must serialize resolved_at as an explicit null.
	raw := do(t, h, http.MethodGet, "/api/v1/incidents/inc-2", "")
	if !strings.Contains(string(raw.body), `"resolved_at":null`) {
		t.Errorf("open incident body %s lacks \"resolved_at\":null", raw.body)
	}
}

func TestGetIncidentNotFound(t *testing.T) {
	h := newTestHandler(t)
	expectError(t, do(t, h, http.MethodGet, "/api/v1/incidents/inc-999", ""),
		http.StatusNotFound, "not_found", `incident "inc-999" not found`)
}

func TestCreateIncident(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodPost, "/api/v1/incidents",
		`{"title":"  DB failover  ","description":"Primary down","service_id":"db","severity":"sev1"}`)
	if res.code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", res.code, res.body)
	}
	inc := decode[wireIncident](t, res)
	if inc.ID != "inc-3" || inc.Title != "DB failover" || inc.Status != "investigating" || inc.ResolvedAt != nil {
		t.Errorf("incident = %+v", inc)
	}
	if len(inc.Updates) != 1 || inc.Updates[0].Message != "Incident declared" || inc.Updates[0].Status != "investigating" {
		t.Errorf("updates = %+v, want one 'Incident declared'", inc.Updates)
	}
	if loc := res.header.Get("Location"); loc != "/api/v1/incidents/inc-3" {
		t.Errorf("Location = %q", loc)
	}

	// The new incident is visible and drives the derived status.
	if got := strings.Join(listIDs(t, h, "/api/v1/incidents?status=open"), ","); got != "inc-3,inc-2" {
		t.Errorf("open incidents = [%s], want [inc-3,inc-2]", got)
	}
	sum := decode[wireSummary](t, do(t, h, http.MethodGet, "/api/v1/summary", ""))
	if sum.OverallStatus != "major_outage" || sum.OpenIncidents != 2 {
		t.Errorf("summary = %+v, want major_outage with 2 open", sum)
	}
}

func TestCreateIncidentDescriptionOptional(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodPost, "/api/v1/incidents", `{"title":"t","service_id":"api","severity":"sev4"}`)
	if res.code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", res.code, res.body)
	}
}

func TestCreateIncidentErrors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"empty body", "", "request body is required"},
		{"malformed JSON", `{"title":`, "malformed JSON"},
		{"syntax error", `{"title" "x"}`, "malformed JSON at offset"},
		{"not an object", `["a"]`, "request body must be a JSON object"},
		{"wrong field type", `{"title":5,"service_id":"api","severity":"sev1"}`, `field "title" must be a string`},
		{"unknown field", `{"title":"t","service_id":"api","severity":"sev1","priority":1}`, `unknown field "priority"`},
		{"trailing data", `{"title":"t","service_id":"api","severity":"sev1"} {}`, "single JSON object"},
		{"too large", `{"title":"t","service_id":"api","severity":"sev1","description":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, "must not exceed 65536 bytes"},
		{"too large trailing data", `{"title":"t","service_id":"api","severity":"sev1"}` + strings.Repeat(" ", MaxBodyBytes), "must not exceed 65536 bytes"},
		{"missing title", `{"service_id":"api","severity":"sev1"}`, "title is required"},
		{"title too long", `{"title":"` + strings.Repeat("a", 121) + `","service_id":"api","severity":"sev1"}`, "title must be at most 120 characters"},
		{"description too long", `{"title":"t","description":"` + strings.Repeat("a", 2001) + `","service_id":"api","severity":"sev1"}`, "description must be at most 2000 characters"},
		{"missing service", `{"title":"t","severity":"sev1"}`, "service_id is required"},
		{"unknown service", `{"title":"t","service_id":"nope","severity":"sev1"}`, `unknown service_id "nope"`},
		{"missing severity", `{"title":"t","service_id":"api"}`, "severity is required"},
		{"bad severity", `{"title":"t","service_id":"api","severity":"critical"}`, "severity must be one of"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			expectError(t, do(t, h, http.MethodPost, "/api/v1/incidents", tc.body),
				http.StatusBadRequest, "invalid_argument", tc.wantMsg)
			if got := len(listIDs(t, h, "/api/v1/incidents")); got != 2 {
				t.Errorf("%d incidents after rejected create, want 2", got)
			}
		})
	}
}

// TestPostRequiresJSONContentType guards against cross-site writes: browsers
// send text/plain and form bodies cross-origin without a CORS preflight.
func TestPostRequiresJSONContentType(t *testing.T) {
	endpoints := []struct {
		name, target, body string
		wantOK             int
	}{
		{"create", "/api/v1/incidents", `{"title":"t","service_id":"api","severity":"sev1"}`, http.StatusCreated},
		{"update", "/api/v1/incidents/inc-2/updates", `{"status":"monitoring","message":"m"}`, http.StatusOK},
	}
	contentTypes := []struct {
		contentType string
		ok          bool
	}{
		{"", false},
		{"text/plain", false},
		{"text/plain;charset=UTF-8", false},
		{"application/x-www-form-urlencoded", false},
		{"multipart/form-data; boundary=x", false},
		{"application/jsonp", false},
		{"application/json; charset", false}, // malformed parameters
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"Application/JSON", true}, // media types are case-insensitive
	}
	for _, ep := range endpoints {
		for _, ct := range contentTypes {
			t.Run(ep.name+"/"+ct.contentType, func(t *testing.T) {
				h := newTestHandler(t)
				res := doWithContentType(t, h, http.MethodPost, ep.target, ct.contentType, ep.body)
				if ct.ok {
					if res.code != ep.wantOK {
						t.Errorf("status = %d, want %d (body %s)", res.code, ep.wantOK, res.body)
					}
					return
				}
				expectError(t, res, http.StatusBadRequest, "invalid_argument", "Content-Type must be application/json")
				// The rejected request must not have changed anything.
				if got := strings.Join(listIDs(t, h, "/api/v1/incidents?status=open"), ","); got != "inc-2" {
					t.Errorf("open incidents = [%s], want [inc-2]", got)
				}
				if inc := decode[wireIncident](t, do(t, h, http.MethodGet, "/api/v1/incidents/inc-2", "")); inc.Status != "identified" {
					t.Errorf("inc-2 status = %q, want unchanged %q", inc.Status, "identified")
				}
			})
		}
	}
}

func TestAddUpdateFlow(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodPost, "/api/v1/incidents/inc-2/updates", `{"status":"monitoring","message":"Fix deployed"}`)
	if res.code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", res.code, res.body)
	}
	inc := decode[wireIncident](t, res)
	last := inc.Updates[len(inc.Updates)-1]
	if inc.Status != "monitoring" || last.Status != "monitoring" || last.Message != "Fix deployed" || inc.ResolvedAt != nil {
		t.Errorf("incident = %+v", inc)
	}

	res = do(t, h, http.MethodPost, "/api/v1/incidents/inc-2/updates", `{"status":"resolved","message":"All clear"}`)
	if res.code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", res.code, res.body)
	}
	inc = decode[wireIncident](t, res)
	if inc.Status != "resolved" || inc.ResolvedAt == nil || *inc.ResolvedAt != "2026-10-02T12:00:00Z" {
		t.Errorf("resolved incident = %+v", inc)
	}

	// Payments returns to operational once its only incident is resolved.
	sum := decode[wireSummary](t, do(t, h, http.MethodGet, "/api/v1/summary", ""))
	if sum.OverallStatus != "operational" || sum.OpenIncidents != 0 {
		t.Errorf("summary = %+v, want operational with 0 open", sum)
	}

	expectError(t, do(t, h, http.MethodPost, "/api/v1/incidents/inc-2/updates", `{"status":"investigating","message":"again"}`),
		http.StatusConflict, "failed_precondition", "already resolved")
}

func TestAddUpdateErrors(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		body       string
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"resolved incident", "inc-1", `{"status":"monitoring","message":"m"}`, http.StatusConflict, "failed_precondition", `incident "inc-1" is already resolved`},
		{"unknown incident", "inc-999", `{"status":"monitoring","message":"m"}`, http.StatusNotFound, "not_found", `incident "inc-999" not found`},
		{"missing status", "inc-2", `{"message":"m"}`, http.StatusBadRequest, "invalid_argument", "status is required"},
		{"bad status", "inc-2", `{"status":"done","message":"m"}`, http.StatusBadRequest, "invalid_argument", "status must be one of"},
		{"missing message", "inc-2", `{"status":"monitoring"}`, http.StatusBadRequest, "invalid_argument", "message is required"},
		{"message too long", "inc-2", `{"status":"monitoring","message":"` + strings.Repeat("m", 1001) + `"}`, http.StatusBadRequest, "invalid_argument", "message must be at most 1000 characters"},
		{"unknown field", "inc-2", `{"status":"monitoring","message":"m","severity":"sev1"}`, http.StatusBadRequest, "invalid_argument", `unknown field "severity"`},
		{"malformed JSON", "inc-2", `{`, http.StatusBadRequest, "invalid_argument", "malformed JSON"},
		{"empty body", "inc-2", ``, http.StatusBadRequest, "invalid_argument", "request body is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			expectError(t, do(t, h, http.MethodPost, "/api/v1/incidents/"+tc.id+"/updates", tc.body),
				tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	tests := []struct {
		method, target, wantAllow string
	}{
		{http.MethodPost, "/healthz", "GET, HEAD"},
		{http.MethodPost, "/api/v1/summary", "GET"},
		{http.MethodDelete, "/api/v1/services", "GET"},
		{http.MethodPut, "/api/v1/incidents", "GET, POST"},
		{http.MethodDelete, "/api/v1/incidents/inc-1", "GET"},
		{http.MethodPost, "/api/v1/incidents/inc-1", "GET"},
		{http.MethodGet, "/api/v1/incidents/inc-1/updates", "POST"},
	}
	h := newTestHandler(t)
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			res := do(t, h, tc.method, tc.target, "")
			expectError(t, res, http.StatusMethodNotAllowed, "method_not_allowed", "method "+tc.method+" not allowed")
			if got := res.header.Get("Allow"); got != tc.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tc.wantAllow)
			}
		})
	}
}

func TestUnknownAPIRoute(t *testing.T) {
	h := newTestHandler(t)
	for _, target := range []string{"/api", "/api/v1/nope", "/api/v2/incidents", "/api/v1/incidents/inc-1/updates/x", "/api/v1/incidents/"} {
		expectError(t, do(t, h, http.MethodGet, target, ""), http.StatusNotFound, "not_found", "no route")
	}
}

func TestStaticUIServedAtRoot(t *testing.T) {
	h := newTestHandler(t)
	res := do(t, h, http.MethodGet, "/", "")
	if res.code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", res.code)
	}
	if !strings.Contains(string(res.body), `data-testid="overall-status"`) {
		t.Errorf("GET / body = %q, want index.html", res.body)
	}
	if ct := res.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if res := do(t, h, http.MethodGet, "/missing.js", ""); res.code != http.StatusNotFound {
		t.Errorf("GET /missing.js status = %d, want 404", res.code)
	}
}

func TestStaticHardening(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"index.html":      indexHTML,
		"app.js":          "export {};",
		"assets/logo.txt": "logo",
		"docs/index.html": "<p>docs</p>",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := New(store.NewSeeded(func() time.Time { return fixedNow }), dir, slog.New(slog.NewTextHandler(io.Discard, nil)))

	tests := []struct {
		target   string
		wantCode int
		wantBody string
	}{
		{"/", http.StatusOK, "overall-status"},
		{"/app.js", http.StatusOK, "export"},
		{"/assets/logo.txt", http.StatusOK, "logo"},
		{"/docs/", http.StatusOK, "<p>docs</p>"},
		{"/assets/", http.StatusNotFound, ""}, // no index.html: no listing
		{"/nope/", http.StatusNotFound, ""},
	}
	for _, tc := range tests {
		res := do(t, h, http.MethodGet, tc.target, "")
		if res.code != tc.wantCode {
			t.Errorf("GET %s status = %d, want %d (body %q)", tc.target, res.code, tc.wantCode, res.body)
		}
		if tc.wantBody != "" && !strings.Contains(string(res.body), tc.wantBody) {
			t.Errorf("GET %s body = %q, want it to contain %q", tc.target, res.body, tc.wantBody)
		}
		if strings.Contains(string(res.body), "<pre>") {
			t.Errorf("GET %s returned a directory listing", tc.target)
		}
		if got := res.header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s X-Content-Type-Options = %q, want nosniff", tc.target, got)
		}
	}
}

var wantSecurityHeaders = map[string]string{
	"X-Content-Type-Options":  "nosniff",
	"Content-Security-Policy": "frame-ancestors 'none'",
	"X-Frame-Options":         "DENY",
}

func checkSecurityHeaders(t *testing.T, what string, h http.Header) {
	t.Helper()
	for name, want := range wantSecurityHeaders {
		if got := h.Values(name); len(got) != 1 || got[0] != want {
			t.Errorf("%s: %s = %q, want exactly [%q]", what, name, got, want)
		}
	}
}

// TestSecurityHeadersOnAllResponses checks the clickjacking and nosniff
// headers on every kind of response: success, each error class, and static.
func TestSecurityHeadersOnAllResponses(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		method, target, body string
		wantCode             int
	}{
		{http.MethodGet, "/healthz", "", http.StatusOK},
		{http.MethodHead, "/healthz", "", http.StatusOK},
		{http.MethodGet, "/api/v1/summary", "", http.StatusOK},
		{http.MethodGet, "/api/v1/services", "", http.StatusOK},
		{http.MethodGet, "/api/v1/incidents?status=open", "", http.StatusOK},
		{http.MethodGet, "/api/v1/incidents/inc-1", "", http.StatusOK},
		{http.MethodPost, "/api/v1/incidents", `{"title":"t","service_id":"api","severity":"sev4"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/incidents/inc-2/updates", `{"status":"monitoring","message":"m"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/incidents", `{}`, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/incidents/inc-999", "", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/incidents", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/incidents/inc-1/updates", `{"status":"monitoring","message":"m"}`, http.StatusConflict},
		{http.MethodGet, "/api/nope", "", http.StatusNotFound},
		{http.MethodGet, "/", "", http.StatusOK},
		{http.MethodGet, "/missing.js", "", http.StatusNotFound},
		{http.MethodGet, "/index.html", "", http.StatusMovedPermanently}, // FileServer redirect to /
	}
	for _, tc := range tests {
		what := tc.method + " " + tc.target
		res := do(t, h, tc.method, tc.target, tc.body)
		if res.code != tc.wantCode {
			t.Errorf("%s: status = %d, want %d", what, res.code, tc.wantCode)
		}
		checkSecurityHeaders(t, what, res.header)
	}

	t.Run("raw text/plain POST rejected", func(t *testing.T) {
		res := doWithContentType(t, h, http.MethodPost, "/api/v1/incidents", "text/plain", `{}`)
		checkSecurityHeaders(t, "text/plain POST", res.header)
	})

	t.Run("recovered panic", func(t *testing.T) {
		panicking := securityHeaders(recoverPanics(slog.New(slog.NewTextHandler(io.Discard, nil)),
			http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })))
		res := do(t, panicking, http.MethodGet, "/", "")
		if res.code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", res.code)
		}
		checkSecurityHeaders(t, "panic", res.header)
	})
}

func TestRecoverPanics(t *testing.T) {
	h := recoverPanics(slog.New(slog.NewTextHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	expectError(t, do(t, h, http.MethodGet, "/", ""), http.StatusInternalServerError, "internal", "internal server error")
}

func TestWriteStoreErrorUnknownErrorIsInternal(t *testing.T) {
	s := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	s.writeStoreError(rec, io.ErrUnexpectedEOF)
	expectError(t, response{rec.Code, rec.Header(), rec.Body.Bytes()}, http.StatusInternalServerError, "internal", "internal server error")
}

func TestWriteJSONEncodingFailureIsInternal(t *testing.T) {
	s := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	s.writeJSON(rec, http.StatusOK, map[string]any{"bad": make(chan int)})
	expectError(t, response{rec.Code, rec.Header(), rec.Body.Bytes()}, http.StatusInternalServerError, "internal", "internal server error")
}
