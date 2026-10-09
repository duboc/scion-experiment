package store

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for deterministic timestamps.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newSeeded(t *testing.T) (*Store, *fakeClock) {
	t.Helper()
	clk := newFakeClock()
	return NewSeeded(clk.Now), clk
}

func mustCreate(t *testing.T, s *Store, in CreateInput) Incident {
	t.Helper()
	inc, err := s.CreateIncident(in)
	if err != nil {
		t.Fatalf("CreateIncident(%+v) error: %v", in, err)
	}
	return inc
}

func serviceStatuses(s *Store) map[string]ServiceStatus {
	out := make(map[string]ServiceStatus)
	for _, svc := range s.Services() {
		out[svc.ID] = svc.Status
	}
	return out
}

func TestSeedData(t *testing.T) {
	s, clk := newSeeded(t)

	svcs := s.Services()
	wantIDs := []string{"api", "auth", "db", "payments", "web"}
	if len(svcs) != len(wantIDs) {
		t.Fatalf("got %d services, want %d", len(svcs), len(wantIDs))
	}
	for i, id := range wantIDs {
		if svcs[i].ID != id {
			t.Errorf("services[%d].ID = %q, want %q (sorted by id)", i, svcs[i].ID, id)
		}
	}
	if got := serviceStatuses(s)["payments"]; got != PartialOutage {
		t.Errorf("payments status = %q, want %q", got, PartialOutage)
	}
	if got := serviceStatuses(s)["web"]; got != Operational {
		t.Errorf("web status = %q, want %q (its seed incident is resolved)", got, Operational)
	}

	sum := s.Summary()
	if sum.OverallStatus != PartialOutage || sum.OpenIncidents != 1 {
		t.Errorf("Summary() = {%q, %d}, want {%q, 1}", sum.OverallStatus, sum.OpenIncidents, PartialOutage)
	}

	open, err := s.ListIncidents(ListFilter{Status: FilterOpen})
	if err != nil || len(open) != 1 || open[0].ServiceID != "payments" || open[0].Severity != Sev2 {
		t.Errorf("open seed incidents = %+v, %v; want one sev2 on payments", open, err)
	}
	resolved, err := s.ListIncidents(ListFilter{Status: FilterResolved})
	if err != nil || len(resolved) != 1 || resolved[0].ServiceID != "web" || resolved[0].Severity != Sev3 {
		t.Fatalf("resolved seed incidents = %+v, %v; want one sev3 on web", resolved, err)
	}
	r := resolved[0]
	if r.ResolvedAt == nil || r.ResolvedAt.After(clk.Now()) {
		t.Errorf("resolved seed ResolvedAt = %v, want non-nil and in the past", r.ResolvedAt)
	}
	for _, inc := range append(open, resolved...) {
		if len(inc.Updates) == 0 || inc.Updates[0].Message != declaredMessage {
			t.Errorf("%s: first update = %+v, want %q", inc.ID, inc.Updates, declaredMessage)
		}
	}
}

func TestEmptyStoreSummaryIsOperational(t *testing.T) {
	s := New(newFakeClock().Now)
	sum := s.Summary()
	if sum.OverallStatus != Operational || sum.OpenIncidents != 0 || len(sum.Services) != 0 {
		t.Errorf("Summary() = %+v, want operational with no services", sum)
	}
	if sum.Services == nil {
		t.Error("Summary().Services is nil, want empty slice (serializes as [])")
	}
}

func TestServiceStatusDerivedFromWorstOpenSeverity(t *testing.T) {
	tests := []struct {
		severities []Severity
		want       ServiceStatus
	}{
		{nil, Operational},
		{[]Severity{Sev4}, Degraded},
		{[]Severity{Sev3}, Degraded},
		{[]Severity{Sev2}, PartialOutage},
		{[]Severity{Sev1}, MajorOutage},
		{[]Severity{Sev4, Sev2, Sev3}, PartialOutage},
		{[]Severity{Sev3, Sev1, Sev2}, MajorOutage},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.severities), func(t *testing.T) {
			s, _ := newSeeded(t)
			for _, sev := range tc.severities {
				mustCreate(t, s, CreateInput{Title: "x", ServiceID: "db", Severity: sev})
			}
			if got := serviceStatuses(s)["db"]; got != tc.want {
				t.Errorf("db status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolvedIncidentsDoNotAffectStatus(t *testing.T) {
	s, _ := newSeeded(t)
	sev1 := mustCreate(t, s, CreateInput{Title: "outage", ServiceID: "db", Severity: Sev1})
	mustCreate(t, s, CreateInput{Title: "minor", ServiceID: "db", Severity: Sev4})
	if _, err := s.AddUpdate(sev1.ID, UpdateInput{Status: StatusResolved, Message: "fixed"}); err != nil {
		t.Fatal(err)
	}
	if got := serviceStatuses(s)["db"]; got != Degraded {
		t.Errorf("db status = %q, want %q after resolving the sev1", got, Degraded)
	}
}

func TestOverallStatusIsWorstServiceStatus(t *testing.T) {
	s, _ := newSeeded(t) // payments: partial_outage
	mustCreate(t, s, CreateInput{Title: "slow", ServiceID: "api", Severity: Sev3})
	if got := s.Summary().OverallStatus; got != PartialOutage {
		t.Errorf("overall = %q, want %q", got, PartialOutage)
	}
	mustCreate(t, s, CreateInput{Title: "down", ServiceID: "auth", Severity: Sev1})
	sum := s.Summary()
	if sum.OverallStatus != MajorOutage {
		t.Errorf("overall = %q, want %q", sum.OverallStatus, MajorOutage)
	}
	if sum.OpenIncidents != 3 {
		t.Errorf("open incidents = %d, want 3", sum.OpenIncidents)
	}
}

func TestCreateIncident(t *testing.T) {
	s, clk := newSeeded(t)
	clk.Advance(1500 * time.Millisecond) // timestamps are truncated to seconds
	inc := mustCreate(t, s, CreateInput{
		Title:       "  Login errors  ",
		Description: " Users see 500s ",
		ServiceID:   "auth",
		Severity:    Sev2,
	})

	want := time.Date(2026, 10, 2, 12, 0, 1, 0, time.UTC)
	if inc.ID != "inc-3" {
		t.Errorf("ID = %q, want inc-3 (after 2 seed incidents)", inc.ID)
	}
	if inc.Title != "Login errors" || inc.Description != "Users see 500s" {
		t.Errorf("title/description = %q/%q, want trimmed", inc.Title, inc.Description)
	}
	if inc.Status != StatusInvestigating || inc.Severity != Sev2 || inc.ServiceID != "auth" {
		t.Errorf("got %+v", inc)
	}
	if !inc.CreatedAt.Equal(want) || !inc.UpdatedAt.Equal(want) || inc.ResolvedAt != nil {
		t.Errorf("times = %v/%v/%v, want %v/%v/nil", inc.CreatedAt, inc.UpdatedAt, inc.ResolvedAt, want, want)
	}
	if inc.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", inc.CreatedAt.Location())
	}
	wantUpdate := Update{Status: StatusInvestigating, Message: "Incident declared", CreatedAt: want}
	if len(inc.Updates) != 1 || inc.Updates[0] != wantUpdate {
		t.Errorf("Updates = %+v, want [%+v]", inc.Updates, wantUpdate)
	}

	got, err := s.GetIncident(inc.ID)
	if err != nil || got.ID != inc.ID {
		t.Errorf("GetIncident(%q) = %+v, %v", inc.ID, got, err)
	}
}

func TestCreateIncidentIDsAreMonotonic(t *testing.T) {
	s, _ := newSeeded(t)
	for i := 3; i <= 6; i++ {
		inc := mustCreate(t, s, CreateInput{Title: "t", ServiceID: "api", Severity: Sev4})
		if want := fmt.Sprintf("inc-%d", i); inc.ID != want {
			t.Errorf("ID = %q, want %q", inc.ID, want)
		}
	}
}

func TestCreateIncidentValidation(t *testing.T) {
	valid := CreateInput{Title: "t", ServiceID: "api", Severity: Sev3}
	tests := []struct {
		name    string
		mutate  func(*CreateInput)
		wantMsg string // empty means success
	}{
		{"valid", func(*CreateInput) {}, ""},
		{"missing title", func(in *CreateInput) { in.Title = "" }, "title is required"},
		{"blank title", func(in *CreateInput) { in.Title = " \t\n " }, "title is required"},
		{"title at limit (multibyte)", func(in *CreateInput) { in.Title = strings.Repeat("é", MaxTitleLen) }, ""},
		{"title too long", func(in *CreateInput) { in.Title = strings.Repeat("a", MaxTitleLen+1) }, "title must be at most 120 characters"},
		{"description at limit", func(in *CreateInput) { in.Description = strings.Repeat("d", MaxDescriptionLen) }, ""},
		{"description too long", func(in *CreateInput) { in.Description = strings.Repeat("d", MaxDescriptionLen+1) }, "description must be at most 2000 characters"},
		{"missing service", func(in *CreateInput) { in.ServiceID = "" }, "service_id is required"},
		{"unknown service", func(in *CreateInput) { in.ServiceID = "nope" }, `unknown service_id "nope"`},
		{"missing severity", func(in *CreateInput) { in.Severity = "" }, "severity is required"},
		{"bad severity", func(in *CreateInput) { in.Severity = "sev5" }, "severity must be one of sev1, sev2, sev3, sev4"},
		{"severity is case sensitive", func(in *CreateInput) { in.Severity = "SEV1" }, "severity must be one of sev1, sev2, sev3, sev4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newSeeded(t)
			in := valid
			tc.mutate(&in)
			_, err := s.CreateIncident(in)
			if tc.wantMsg == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidArgument) || err.Error() != tc.wantMsg {
				t.Fatalf("error = %v, want invalid argument %q", err, tc.wantMsg)
			}
			if n := len(mustList(t, s, ListFilter{})); n != 2 {
				t.Errorf("store has %d incidents after failed create, want 2", n)
			}
		})
	}
}

func mustList(t *testing.T, s *Store, f ListFilter) []Incident {
	t.Helper()
	incs, err := s.ListIncidents(f)
	if err != nil {
		t.Fatalf("ListIncidents(%+v) error: %v", f, err)
	}
	return incs
}

func ids(incs []Incident) []string {
	out := make([]string, len(incs))
	for i, inc := range incs {
		out[i] = inc.ID
	}
	return out
}

func TestListIncidentsNewestFirst(t *testing.T) {
	s, clk := newSeeded(t)                                                      // seeds: inc-1 created 3h ago, inc-2 45m ago
	mustCreate(t, s, CreateInput{Title: "a", ServiceID: "api", Severity: Sev3}) // inc-3
	mustCreate(t, s, CreateInput{Title: "b", ServiceID: "api", Severity: Sev3}) // inc-4, same second
	clk.Advance(time.Minute)
	mustCreate(t, s, CreateInput{Title: "c", ServiceID: "db", Severity: Sev3}) // inc-5

	got := fmt.Sprint(ids(mustList(t, s, ListFilter{})))
	if want := "[inc-5 inc-4 inc-3 inc-2 inc-1]"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}

func TestListIncidentsFilters(t *testing.T) {
	s, _ := newSeeded(t)
	mustCreate(t, s, CreateInput{Title: "a", ServiceID: "web", Severity: Sev3}) // inc-3, open on web

	tests := []struct {
		filter ListFilter
		want   string
	}{
		{ListFilter{}, "[inc-3 inc-2 inc-1]"},
		{ListFilter{Status: FilterOpen}, "[inc-3 inc-2]"},
		{ListFilter{Status: FilterResolved}, "[inc-1]"},
		{ListFilter{ServiceID: "web"}, "[inc-3 inc-1]"},
		{ListFilter{ServiceID: "web", Status: FilterOpen}, "[inc-3]"},
		{ListFilter{ServiceID: "db"}, "[]"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%+v", tc.filter), func(t *testing.T) {
			incs := mustList(t, s, tc.filter)
			if incs == nil {
				t.Error("ListIncidents returned nil, want empty slice")
			}
			if got := fmt.Sprint(ids(incs)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestListIncidentsInvalidFilters(t *testing.T) {
	s, _ := newSeeded(t)
	tests := []struct {
		filter  ListFilter
		wantMsg string
	}{
		{ListFilter{Status: "closed"}, `status filter must be "open" or "resolved"`},
		{ListFilter{Status: "OPEN"}, `status filter must be "open" or "resolved"`},
		{ListFilter{ServiceID: "nope"}, `unknown service_id "nope"`},
	}
	for _, tc := range tests {
		_, err := s.ListIncidents(tc.filter)
		if !errors.Is(err, ErrInvalidArgument) || err.Error() != tc.wantMsg {
			t.Errorf("ListIncidents(%+v) error = %v, want invalid argument %q", tc.filter, err, tc.wantMsg)
		}
	}
}

func TestGetIncidentNotFound(t *testing.T) {
	s, _ := newSeeded(t)
	_, err := s.GetIncident("inc-999")
	if !errors.Is(err, ErrNotFound) || err.Error() != `incident "inc-999" not found` {
		t.Errorf("error = %v, want not found", err)
	}
}

func TestAddUpdateLifecycle(t *testing.T) {
	s, clk := newSeeded(t)
	inc := mustCreate(t, s, CreateInput{Title: "t", ServiceID: "db", Severity: Sev1})

	steps := []Status{StatusIdentified, StatusMonitoring, StatusResolved}
	for i, st := range steps {
		clk.Advance(time.Minute)
		got, err := s.AddUpdate(inc.ID, UpdateInput{Status: st, Message: fmt.Sprintf("  step %d  ", i)})
		if err != nil {
			t.Fatalf("AddUpdate(%s) error: %v", st, err)
		}
		now := clk.Now()
		if got.Status != st || !got.UpdatedAt.Equal(now) {
			t.Errorf("after %s: status=%q updated_at=%v, want %q/%v", st, got.Status, got.UpdatedAt, st, now)
		}
		if len(got.Updates) != i+2 {
			t.Fatalf("after %s: %d updates, want %d", st, len(got.Updates), i+2)
		}
		last := got.Updates[len(got.Updates)-1]
		if want := (Update{Status: st, Message: fmt.Sprintf("step %d", i), CreatedAt: now}); last != want {
			t.Errorf("last update = %+v, want %+v", last, want)
		}
		if st == StatusResolved {
			if got.ResolvedAt == nil || !got.ResolvedAt.Equal(now) {
				t.Errorf("ResolvedAt = %v, want %v", got.ResolvedAt, now)
			}
		} else if got.ResolvedAt != nil {
			t.Errorf("ResolvedAt = %v before resolution, want nil", got.ResolvedAt)
		}
	}

	if got := serviceStatuses(s)["db"]; got != Operational {
		t.Errorf("db status = %q after resolve, want operational", got)
	}
}

func TestAddUpdateCanMoveBackwardsBeforeResolution(t *testing.T) {
	s, _ := newSeeded(t)
	got, err := s.AddUpdate("inc-2", UpdateInput{Status: StatusInvestigating, Message: "fix did not hold"})
	if err != nil || got.Status != StatusInvestigating {
		t.Errorf("AddUpdate = %q, %v; want investigating, nil", got.Status, err)
	}
}

func TestAddUpdateResolvedIsFailedPrecondition(t *testing.T) {
	s, _ := newSeeded(t)
	before, _ := s.GetIncident("inc-1") // resolved seed incident
	_, err := s.AddUpdate("inc-1", UpdateInput{Status: StatusInvestigating, Message: "reopen"})
	if !errors.Is(err, ErrFailedPrecondition) || err.Error() != `incident "inc-1" is already resolved` {
		t.Fatalf("error = %v, want failed precondition", err)
	}
	after, _ := s.GetIncident("inc-1")
	if len(after.Updates) != len(before.Updates) || after.Status != StatusResolved {
		t.Errorf("incident changed after rejected update: %+v", after)
	}
}

func TestAddUpdateValidation(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		in      UpdateInput
		wantErr error
		wantMsg string
	}{
		{"missing status", "inc-2", UpdateInput{Message: "m"}, ErrInvalidArgument, "status is required"},
		{"bad status", "inc-2", UpdateInput{Status: "fixed", Message: "m"}, ErrInvalidArgument, "status must be one of investigating, identified, monitoring, resolved"},
		{"missing message", "inc-2", UpdateInput{Status: StatusMonitoring}, ErrInvalidArgument, "message is required"},
		{"blank message", "inc-2", UpdateInput{Status: StatusMonitoring, Message: "   "}, ErrInvalidArgument, "message is required"},
		{"message too long", "inc-2", UpdateInput{Status: StatusMonitoring, Message: strings.Repeat("m", MaxMessageLen+1)}, ErrInvalidArgument, "message must be at most 1000 characters"},
		{"unknown incident", "inc-999", UpdateInput{Status: StatusMonitoring, Message: "m"}, ErrNotFound, `incident "inc-999" not found`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newSeeded(t)
			_, err := s.AddUpdate(tc.id, tc.in)
			if !errors.Is(err, tc.wantErr) || err.Error() != tc.wantMsg {
				t.Errorf("error = %v, want %v %q", err, tc.wantErr, tc.wantMsg)
			}
		})
	}

	t.Run("message at limit", func(t *testing.T) {
		s, _ := newSeeded(t)
		if _, err := s.AddUpdate("inc-2", UpdateInput{Status: StatusMonitoring, Message: strings.Repeat("ü", MaxMessageLen)}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestReturnedIncidentsAreCopies(t *testing.T) {
	s, _ := newSeeded(t)
	inc, _ := s.GetIncident("inc-1")
	inc.Updates[0].Message = "tampered"
	*inc.ResolvedAt = time.Time{}
	inc.Title = "tampered"

	again, _ := s.GetIncident("inc-1")
	if again.Updates[0].Message == "tampered" || again.ResolvedAt.IsZero() || again.Title == "tampered" {
		t.Errorf("mutating a returned incident changed the store: %+v", again)
	}
}

func TestErrorIs(t *testing.T) {
	err := notFound("x")
	if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidArgument) || errors.Is(err, errors.New("not found")) {
		t.Error("errors.Is must match by Code against *Error only")
	}
}

// TestConcurrentAccess exercises the store from many goroutines; run with
// -race to detect data races.
func TestConcurrentAccess(t *testing.T) {
	s, _ := newSeeded(t)
	const workers, perWorker = 8, 25

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				inc, err := s.CreateIncident(CreateInput{Title: "t", ServiceID: "api", Severity: Sev3})
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := s.AddUpdate(inc.ID, UpdateInput{Status: StatusResolved, Message: "done"}); err != nil {
					t.Error(err)
					return
				}
				_ = s.Summary()
				if _, err := s.ListIncidents(ListFilter{Status: FilterOpen}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()

	all := mustList(t, s, ListFilter{})
	if want := 2 + workers*perWorker; len(all) != want {
		t.Errorf("got %d incidents, want %d", len(all), want)
	}
	seen := make(map[string]bool)
	for _, inc := range all {
		if seen[inc.ID] {
			t.Errorf("duplicate ID %s", inc.ID)
		}
		seen[inc.ID] = true
	}
	if sum := s.Summary(); sum.OpenIncidents != 1 {
		t.Errorf("open incidents = %d, want 1 (only the seed)", sum.OpenIncidents)
	}
}
