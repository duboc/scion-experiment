// Package store implements the in-memory, concurrency-safe incident store
// described in DESIGN.md §6.
package store

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Field length limits from DESIGN.md §6 and §7, measured in characters (runes).
const (
	MaxTitleLen       = 120
	MaxDescriptionLen = 2000
	MaxMessageLen     = 1000
)

// declaredMessage is the message on the first update of every new incident.
const declaredMessage = "Incident declared"

// Severity is the impact level of an incident.
type Severity string

// Valid severities, from most to least severe.
const (
	Sev1 Severity = "sev1"
	Sev2 Severity = "sev2"
	Sev3 Severity = "sev3"
	Sev4 Severity = "sev4"
)

func (s Severity) valid() bool {
	switch s {
	case Sev1, Sev2, Sev3, Sev4:
		return true
	}
	return false
}

// impact maps a severity to the service status it causes.
func (s Severity) impact() ServiceStatus {
	switch s {
	case Sev1:
		return MajorOutage
	case Sev2:
		return PartialOutage
	default:
		return Degraded
	}
}

// Status is the lifecycle state of an incident.
type Status string

// Valid incident statuses.
const (
	StatusInvestigating Status = "investigating"
	StatusIdentified    Status = "identified"
	StatusMonitoring    Status = "monitoring"
	StatusResolved      Status = "resolved"
)

func (s Status) valid() bool {
	switch s {
	case StatusInvestigating, StatusIdentified, StatusMonitoring, StatusResolved:
		return true
	}
	return false
}

// ServiceStatus is the derived health of a service, or of the whole system.
type ServiceStatus string

// Service statuses, ordered from best to worst.
const (
	Operational   ServiceStatus = "operational"
	Degraded      ServiceStatus = "degraded"
	PartialOutage ServiceStatus = "partial_outage"
	MajorOutage   ServiceStatus = "major_outage"
)

func (s ServiceStatus) rank() int {
	switch s {
	case Degraded:
		return 1
	case PartialOutage:
		return 2
	case MajorOutage:
		return 3
	default:
		return 0
	}
}

func worse(a, b ServiceStatus) ServiceStatus {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Service is a monitored service with its derived status.
type Service struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Status ServiceStatus `json:"status"`
}

// Update is one entry in an incident's timeline.
type Update struct {
	Status    Status    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Incident is a declared incident. Values returned by Store are deep copies
// and are safe for callers to keep or modify.
type Incident struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ServiceID   string     `json:"service_id"`
	Severity    Severity   `json:"severity"`
	Status      Status     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
	Updates     []Update   `json:"updates"`

	seq int // numeric part of ID; breaks ordering ties.
}

func (i *Incident) open() bool { return i.Status != StatusResolved }

func (i *Incident) clone() Incident {
	c := *i
	c.Updates = slices.Clone(i.Updates)
	if i.ResolvedAt != nil {
		t := *i.ResolvedAt
		c.ResolvedAt = &t
	}
	return c
}

// Summary is the overall system status.
type Summary struct {
	OverallStatus ServiceStatus `json:"overall_status"`
	OpenIncidents int           `json:"open_incidents"`
	Services      []Service     `json:"services"`
}

// CreateInput is the input to CreateIncident.
type CreateInput struct {
	Title       string
	Description string
	ServiceID   string
	Severity    Severity
}

// UpdateInput is the input to AddUpdate.
type UpdateInput struct {
	Status  Status
	Message string
}

// Values accepted by ListFilter.Status.
const (
	FilterOpen     = "open"
	FilterResolved = "resolved"
)

// ListFilter restricts ListIncidents. Empty fields match everything.
type ListFilter struct {
	Status    string // "", FilterOpen or FilterResolved.
	ServiceID string
}

// Store holds services and incidents in memory. It is safe for concurrent use.
type Store struct {
	now func() time.Time

	mu        sync.RWMutex
	services  map[string]string // id -> display name
	incidents map[string]*Incident
	lastSeq   int
}

// New returns an empty store. now is the clock; pass time.Now in production.
func New(now func() time.Time) *Store {
	return &Store{
		now:       now,
		services:  make(map[string]string),
		incidents: make(map[string]*Incident),
	}
}

// timestamp returns the current time in UTC at second precision, so that it
// serializes as plain RFC3339.
func (s *Store) timestamp() time.Time {
	return s.now().UTC().Truncate(time.Second)
}

// AddService registers a service. Re-adding an id replaces its name.
func (s *Store) AddService(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[id] = name
}

// insert assigns an ID to inc and stores it. Caller must hold s.mu.
func (s *Store) insert(inc Incident) Incident {
	s.lastSeq++
	inc.seq = s.lastSeq
	inc.ID = fmt.Sprintf("inc-%d", inc.seq)
	s.incidents[inc.ID] = &inc
	return inc.clone()
}

// Services returns all services sorted by id, with derived statuses.
func (s *Store) Services() []Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.servicesLocked()
}

func (s *Store) servicesLocked() []Service {
	status := make(map[string]ServiceStatus, len(s.services))
	for _, inc := range s.incidents {
		if inc.open() {
			status[inc.ServiceID] = worse(status[inc.ServiceID], inc.Severity.impact())
		}
	}
	out := make([]Service, 0, len(s.services))
	for id, name := range s.services {
		st := status[id]
		if st == "" {
			st = Operational
		}
		out = append(out, Service{ID: id, Name: name, Status: st})
	}
	slices.SortFunc(out, func(a, b Service) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// Summary returns the overall status, open incident count and services.
func (s *Store) Summary() Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sum := Summary{OverallStatus: Operational, Services: s.servicesLocked()}
	for _, svc := range sum.Services {
		sum.OverallStatus = worse(sum.OverallStatus, svc.Status)
	}
	for _, inc := range s.incidents {
		if inc.open() {
			sum.OpenIncidents++
		}
	}
	return sum
}

// ListIncidents returns matching incidents, newest first.
func (s *Store) ListIncidents(f ListFilter) ([]Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	switch f.Status {
	case "", FilterOpen, FilterResolved:
	default:
		return nil, invalidf("status filter must be %q or %q", FilterOpen, FilterResolved)
	}
	if f.ServiceID != "" {
		if _, ok := s.services[f.ServiceID]; !ok {
			return nil, invalidf("unknown service_id %q", f.ServiceID)
		}
	}

	out := make([]Incident, 0, len(s.incidents))
	for _, inc := range s.incidents {
		if f.Status == FilterOpen && !inc.open() || f.Status == FilterResolved && inc.open() {
			continue
		}
		if f.ServiceID != "" && inc.ServiceID != f.ServiceID {
			continue
		}
		out = append(out, inc.clone())
	}
	slices.SortFunc(out, func(a, b Incident) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.seq, a.seq)
	})
	return out, nil
}

// GetIncident returns the incident with the given id.
func (s *Store) GetIncident(id string) (Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inc, ok := s.incidents[id]
	if !ok {
		return Incident{}, notFound(id)
	}
	return inc.clone(), nil
}

// CreateIncident validates in and declares a new incident in status
// "investigating" with a single "Incident declared" update.
func (s *Store) CreateIncident(in CreateInput) (Incident, error) {
	title := strings.TrimSpace(in.Title)
	desc := strings.TrimSpace(in.Description)

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case title == "":
		return Incident{}, invalidf("title is required")
	case utf8.RuneCountInString(title) > MaxTitleLen:
		return Incident{}, invalidf("title must be at most %d characters", MaxTitleLen)
	case utf8.RuneCountInString(desc) > MaxDescriptionLen:
		return Incident{}, invalidf("description must be at most %d characters", MaxDescriptionLen)
	case in.ServiceID == "":
		return Incident{}, invalidf("service_id is required")
	case in.Severity == "":
		return Incident{}, invalidf("severity is required")
	case !in.Severity.valid():
		return Incident{}, invalidf("severity must be one of sev1, sev2, sev3, sev4")
	}
	if _, ok := s.services[in.ServiceID]; !ok {
		return Incident{}, invalidf("unknown service_id %q", in.ServiceID)
	}

	now := s.timestamp()
	return s.insert(Incident{
		Title:       title,
		Description: desc,
		ServiceID:   in.ServiceID,
		Severity:    in.Severity,
		Status:      StatusInvestigating,
		CreatedAt:   now,
		UpdatedAt:   now,
		Updates:     []Update{{Status: StatusInvestigating, Message: declaredMessage, CreatedAt: now}},
	}), nil
}

// AddUpdate appends a timeline update to an open incident and moves it to
// in.Status. Resolved incidents cannot be updated.
func (s *Store) AddUpdate(id string, in UpdateInput) (Incident, error) {
	msg := strings.TrimSpace(in.Message)
	switch {
	case in.Status == "":
		return Incident{}, invalidf("status is required")
	case !in.Status.valid():
		return Incident{}, invalidf("status must be one of investigating, identified, monitoring, resolved")
	case msg == "":
		return Incident{}, invalidf("message is required")
	case utf8.RuneCountInString(msg) > MaxMessageLen:
		return Incident{}, invalidf("message must be at most %d characters", MaxMessageLen)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inc, ok := s.incidents[id]
	if !ok {
		return Incident{}, notFound(id)
	}
	if !inc.open() {
		return Incident{}, &Error{Code: CodeFailedPrecondition, Message: fmt.Sprintf("incident %q is already resolved", id)}
	}

	now := s.timestamp()
	inc.Status = in.Status
	inc.UpdatedAt = now
	inc.Updates = append(inc.Updates, Update{Status: in.Status, Message: msg, CreatedAt: now})
	if in.Status == StatusResolved {
		inc.ResolvedAt = &now
	}
	return inc.clone(), nil
}
