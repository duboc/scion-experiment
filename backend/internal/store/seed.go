package store

import "time"

// NewSeeded returns a store loaded with the seed data from DESIGN.md §6:
// five services, one open sev2 incident on payments, and one resolved sev3
// incident on web. Seed timestamps are relative to now().
func NewSeeded(now func() time.Time) *Store {
	s := New(now)
	for _, svc := range []struct{ id, name string }{
		{"api", "Public API"},
		{"web", "Web App"},
		{"db", "Database"},
		{"auth", "Authentication"},
		{"payments", "Payments"},
	} {
		s.AddService(svc.id, svc.name)
	}

	t := s.timestamp()
	at := func(ago time.Duration) time.Time { return t.Add(-ago) }

	s.mu.Lock()
	defer s.mu.Unlock()

	resolvedAt := at(2 * time.Hour)
	s.insert(Incident{
		Title:       "Slow page loads on dashboard",
		Description: "Users reported dashboard pages taking more than 10s to load.",
		ServiceID:   "web",
		Severity:    Sev3,
		Status:      StatusResolved,
		CreatedAt:   at(3 * time.Hour),
		UpdatedAt:   resolvedAt,
		ResolvedAt:  &resolvedAt,
		Updates: []Update{
			{Status: StatusInvestigating, Message: declaredMessage, CreatedAt: at(3 * time.Hour)},
			{Status: StatusMonitoring, Message: "Rolled back the frontend release; monitoring latency.", CreatedAt: at(150 * time.Minute)},
			{Status: StatusResolved, Message: "Latency back to normal for 30 minutes.", CreatedAt: resolvedAt},
		},
	})
	s.insert(Incident{
		Title:       "Elevated card payment failures",
		Description: "Around 8% of card payments are failing with upstream timeouts.",
		ServiceID:   "payments",
		Severity:    Sev2,
		Status:      StatusIdentified,
		CreatedAt:   at(45 * time.Minute),
		UpdatedAt:   at(20 * time.Minute),
		Updates: []Update{
			{Status: StatusInvestigating, Message: declaredMessage, CreatedAt: at(45 * time.Minute)},
			{Status: StatusIdentified, Message: "Root cause identified: latency at the upstream card processor.", CreatedAt: at(20 * time.Minute)},
		},
	})
	return s
}
