// Package api implements the HTTP JSON API from DESIGN.md §7 and serves the
// static dashboard UI.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"

	"incidentdash/internal/store"
)

// MaxBodyBytes is the largest accepted request body (DESIGN.md §7).
const MaxBodyBytes = 64 << 10

// Error codes for the error envelope (DESIGN.md §7).
const (
	codeInvalidArgument    = "invalid_argument"
	codeNotFound           = "not_found"
	codeMethodNotAllowed   = "method_not_allowed"
	codeFailedPrecondition = "failed_precondition"
	codeInternal           = "internal"
)

type server struct {
	store  *store.Store
	logger *slog.Logger
}

// New returns a handler serving the API under /api/v1/, /healthz, and the
// static files in staticDir at /.
func New(st *store.Store, staticDir string, logger *slog.Logger) http.Handler {
	s := &server{store: st, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/v1/summary", s.handleSummary)
	mux.HandleFunc("/api/v1/services", s.handleServices)
	mux.HandleFunc("/api/v1/incidents", s.handleIncidents)
	mux.HandleFunc("/api/v1/incidents/{id}", s.handleIncident)
	mux.HandleFunc("/api/v1/incidents/{id}/updates", s.handleIncidentUpdates)
	mux.HandleFunc("/api", handleAPINotFound)
	mux.HandleFunc("/api/", handleAPINotFound)
	mux.Handle("/", staticHandler(staticDir))

	return securityHeaders(recoverPanics(logger, mux))
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// HEAD is allowed for load-balancer style probes; net/http drops the body.
	if !allowMethods(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if !allowMethods(w, r, http.MethodGet) {
		return
	}
	s.writeJSON(w, http.StatusOK, s.store.Summary())
}

func (s *server) handleServices(w http.ResponseWriter, r *http.Request) {
	if !allowMethods(w, r, http.MethodGet) {
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]store.Service{"services": s.store.Services()})
}

func (s *server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listIncidents(w, r)
	case http.MethodPost:
		s.createIncident(w, r)
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *server) listIncidents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	incidents, err := s.store.ListIncidents(store.ListFilter{
		Status:    q.Get("status"),
		ServiceID: q.Get("service_id"),
	})
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]store.Incident{"incidents": incidents})
}

type createIncidentRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ServiceID   string `json:"service_id"`
	Severity    string `json:"severity"`
}

func (s *server) createIncident(w http.ResponseWriter, r *http.Request) {
	var req createIncidentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidArgument, err.Error())
		return
	}
	inc, err := s.store.CreateIncident(store.CreateInput{
		Title:       req.Title,
		Description: req.Description,
		ServiceID:   req.ServiceID,
		Severity:    store.Severity(req.Severity),
	})
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/incidents/"+inc.ID)
	s.writeJSON(w, http.StatusCreated, inc)
}

func (s *server) handleIncident(w http.ResponseWriter, r *http.Request) {
	if !allowMethods(w, r, http.MethodGet) {
		return
	}
	inc, err := s.store.GetIncident(r.PathValue("id"))
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, inc)
}

type addUpdateRequest struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *server) handleIncidentUpdates(w http.ResponseWriter, r *http.Request) {
	if !allowMethods(w, r, http.MethodPost) {
		return
	}
	var req addUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidArgument, err.Error())
		return
	}
	inc, err := s.store.AddUpdate(r.PathValue("id"), store.UpdateInput{
		Status:  store.Status(req.Status),
		Message: req.Message,
	})
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, inc)
}

func handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, codeNotFound, fmt.Sprintf("no route for %s", r.URL.Path))
}

// allowMethods writes a 405 and returns false unless r.Method is allowed.
func allowMethods(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, m := range allowed {
		if r.Method == m {
			return true
		}
	}
	writeMethodNotAllowed(w, r, allowed...)
	return false
}

func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allowed ...string) {
	list := strings.Join(allowed, ", ")
	w.Header().Set("Allow", list)
	writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed,
		fmt.Sprintf("method %s not allowed; allowed: %s", r.Method, list))
}

// decodeJSON decodes a single JSON object from the request body into dst,
// rejecting non-JSON content types, unknown fields, trailing data and bodies
// over MaxBodyBytes. The returned error message is safe to show to clients.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// Requiring application/json makes browsers send a CORS preflight for
	// cross-origin writes. The server never approves preflights, so other
	// sites cannot declare or resolve incidents via "simple" requests.
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return errors.New("Content-Type must be application/json")
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var (
			syntaxErr *json.SyntaxError
			typeErr   *json.UnmarshalTypeError
			tooLarge  *http.MaxBytesError
		)
		switch {
		case errors.As(err, &tooLarge):
			return fmt.Errorf("request body must not exceed %d bytes", MaxBodyBytes)
		case errors.Is(err, io.EOF):
			return errors.New("request body is required")
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("malformed JSON at offset %d", syntaxErr.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("malformed JSON: unexpected end of body")
		case errors.As(err, &typeErr):
			if typeErr.Field == "" {
				return errors.New("request body must be a JSON object")
			}
			return fmt.Errorf("field %q must be a %s", typeErr.Field, typeErr.Type)
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			// encoding/json has no typed error for unknown fields.
			return fmt.Errorf("unknown field %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
		default:
			return errors.New("invalid JSON body")
		}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("request body must not exceed %d bytes", MaxBodyBytes)
		}
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func (s *server) writeStoreError(w http.ResponseWriter, err error) {
	var se *store.Error
	if !errors.As(err, &se) {
		s.logger.Error("unexpected store error", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "internal server error")
		return
	}
	switch se.Code {
	case store.CodeInvalidArgument:
		writeError(w, http.StatusBadRequest, codeInvalidArgument, se.Message)
	case store.CodeNotFound:
		writeError(w, http.StatusNotFound, codeNotFound, se.Message)
	case store.CodeFailedPrecondition:
		writeError(w, http.StatusConflict, codeFailedPrecondition, se.Message)
	default:
		s.logger.Error("unknown store error code", "code", se.Code, "err", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "internal server error")
	}
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	// errorBody always marshals successfully.
	body, _ := json.Marshal(errorBody{Error: errorDetail{Code: code, Message: message}})
	writeBody(w, status, body)
}

func (s *server) writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("encoding response", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternal, "internal server error")
		return
	}
	writeBody(w, status, body)
}

func writeBody(w http.ResponseWriter, status int, body []byte) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n')) // Client disconnects are not actionable.
}

// staticHandler serves files from dir. Directories without an index.html
// return 404 instead of a generated listing.
func staticHandler(dir string) http.Handler {
	root := http.Dir(dir)
	files := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") && !hasIndex(root, r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// hasIndex reports whether the directory at urlPath in root contains an
// index.html file.
func hasIndex(root http.FileSystem, urlPath string) bool {
	f, err := root.Open(path.Join(urlPath, "index.html"))
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	return err == nil && !info.IsDir()
}

// securityHeaders sets hardening headers on every response before any
// handler runs, so errors, panics and static files are covered too:
//   - nosniff stops browsers from MIME-sniffing responses;
//   - CSP frame-ancestors 'none' forbids framing by any origin (clickjacking),
//     with X-Frame-Options: DENY for browsers that predate frame-ancestors.
//
// The CSP header contains only frame-ancestors. Browsers enforce every
// policy present, so it adds to, and never loosens, the frontend's
// <meta http-equiv> CSP (which cannot express frame-ancestors).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// recoverPanics turns handler panics into a 500 error envelope.
func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			logger.Error("panic serving request", "method", r.Method, "path", r.URL.Path, "panic", v)
			writeError(w, http.StatusInternalServerError, codeInternal, "internal server error")
		}()
		next.ServeHTTP(w, r)
	})
}
