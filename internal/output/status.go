package output

import (
	"context"
	"encoding/json"
	"time"

	"github.com/anned20/kimai-cli/internal/kimai"
)

// LookupFunc resolves the entity-name index, which costs several API calls.
type LookupFunc func(context.Context) (*kimai.Lookup, error)

// Status is the template context for the `status` command.
//
// Every expensive field is a method, so Go's text/template evaluates it only
// when the template names it. A format string of "{{.CurrentDuration}}" costs
// the single call that fetched the active entry; naming a project or customer
// additionally resolves the entity index, and naming a daily or weekly total
// additionally queries that window. Each is memoised, so repeating a field in
// one template does not repeat its work.
type Status struct {
	entry  *kimai.Timesheet
	ctx    context.Context
	client *kimai.Client
	lookup LookupFunc
	now    time.Time

	names    *kimai.Lookup
	resolved bool
	daily    *cached
	weekly   *cached
}

type cached struct {
	value string
	err   error
	done  bool
}

// NewStatus builds a status context. entry is nil when the clock is stopped.
func NewStatus(ctx context.Context, c *kimai.Client, lookup LookupFunc, entry *kimai.Timesheet) *Status {
	return &Status{
		entry:  entry,
		ctx:    ctx,
		client: c,
		lookup: lookup,
		now:    time.Now(),
		daily:  &cached{},
		weekly: &cached{},
	}
}

// Running reports whether a timer is currently going.
func (s *Status) Running() bool { return s.entry != nil }

// ID is the running entry's ID, or 0 when stopped.
func (s *Status) ID() int {
	if s.entry == nil {
		return 0
	}
	return s.entry.ID
}

// Description is the running entry's description, empty when stopped.
func (s *Status) Description() string {
	if s.entry == nil {
		return ""
	}
	return s.entry.Description
}

// Billable reports whether the running entry is billable.
func (s *Status) Billable() bool { return s.entry != nil && s.entry.Billable }

// Tags are the running entry's tags.
func (s *Status) Tags() []string {
	if s.entry == nil || s.entry.Tags == nil {
		return []string{}
	}
	return s.entry.Tags
}

// CurrentDuration is how long the running entry has been going, or "0s".
// It costs nothing beyond the call that fetched the entry.
func (s *Status) CurrentDuration() string {
	if s.entry == nil {
		return Duration(0)
	}
	return Duration(s.entry.Elapsed())
}

// resolveNames resolves the entity index once, on first use. The result is
// cached even when it is nil, so a lookup is never repeated.
func (s *Status) resolveNames() (*kimai.Lookup, error) {
	if s.resolved || s.lookup == nil {
		return s.names, nil
	}
	l, err := s.lookup(s.ctx)
	if err != nil {
		return nil, err
	}
	s.names, s.resolved = l, true
	return l, nil
}

// Project is the running entry's project name. Resolving it costs the entity
// index, so a status bar that omits it stays a single request.
func (s *Status) Project() (string, error) {
	if s.entry == nil {
		return "", nil
	}
	if s.entry.Project.Name != "" {
		return s.entry.Project.Name, nil
	}
	l, err := s.resolveNames()
	if err != nil || l == nil {
		return "", err
	}
	return l.ProjectName(s.entry.Project.ID), nil
}

// Customer is the customer owning the running entry's project.
func (s *Status) Customer() (string, error) {
	if s.entry == nil {
		return "", nil
	}
	l, err := s.resolveNames()
	if err != nil || l == nil {
		return "", err
	}
	return l.CustomerName(s.entry.Project.ID), nil
}

// Activity is the running entry's activity name.
func (s *Status) Activity() (string, error) {
	if s.entry == nil {
		return "", nil
	}
	if s.entry.Activity.Name != "" {
		return s.entry.Activity.Name, nil
	}
	l, err := s.resolveNames()
	if err != nil || l == nil {
		return "", err
	}
	return l.ActivityName(s.entry.Activity.ID), nil
}

// DailyDuration totals today's entries. Costs one request, memoised.
func (s *Status) DailyDuration() (string, error) {
	start := time.Date(s.now.Year(), s.now.Month(), s.now.Day(), 0, 0, 0, 0, s.now.Location())
	return s.total(s.daily, start, start.AddDate(0, 0, 1))
}

// WeeklyDuration totals this week's entries from Monday. Costs one request,
// memoised.
func (s *Status) WeeklyDuration() (string, error) {
	weekday := (int(s.now.Weekday()) + 6) % 7 // Monday is 0
	start := time.Date(s.now.Year(), s.now.Month(), s.now.Day(), 0, 0, 0, 0, s.now.Location()).
		AddDate(0, 0, -weekday)
	return s.total(s.weekly, start, start.AddDate(0, 0, 7))
}

// total sums entry durations in a window, counting a running entry up to now.
func (s *Status) total(slot *cached, begin, end time.Time) (string, error) {
	if slot.done {
		return slot.value, slot.err
	}
	slot.done = true

	entries, err := s.client.Timesheets(s.ctx, kimai.TimesheetQuery{Begin: begin, End: end})
	if err != nil {
		slot.err = err
		return "", err
	}
	var sum time.Duration
	for _, t := range entries {
		sum += t.Elapsed()
	}
	slot.value = Duration(sum)
	return slot.value, nil
}

// MarshalJSON renders the status with every field resolved, so --json is a
// complete snapshot while --format stays lazy.
func (s *Status) MarshalJSON() ([]byte, error) {
	daily, err := s.DailyDuration()
	if err != nil {
		return nil, err
	}
	weekly, err := s.WeeklyDuration()
	if err != nil {
		return nil, err
	}

	var active *Entry
	if s.entry != nil {
		names, err := s.resolveNames()
		if err != nil {
			return nil, err
		}
		entry := NewEntry(*s.entry, names)
		active = &entry
	}

	return json.Marshal(struct {
		Running         bool   `json:"running"`
		Active          *Entry `json:"active"`
		CurrentDuration string `json:"current_duration"`
		DailyDuration   string `json:"daily_duration"`
		WeeklyDuration  string `json:"weekly_duration"`
	}{
		Running:         s.Running(),
		Active:          active,
		CurrentDuration: s.CurrentDuration(),
		DailyDuration:   daily,
		WeeklyDuration:  weekly,
	})
}
