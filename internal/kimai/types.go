// Package kimai is a client for the Kimai REST API.
package kimai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Time wraps time.Time to handle Kimai's RFC3339 timestamps, which omit the
// zone offset on some endpoints.
type Time struct{ time.Time }

const kimaiLayout = "2006-01-02T15:04:05"

// Kimai emits numeric offsets without a colon ("+0300"), which RFC3339 does
// not accept, and omits the offset entirely on some endpoints.
var readLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05-0700",
	kimaiLayout,
}

// UnmarshalJSON accepts both offset-bearing and bare Kimai timestamps.
func (t *Time) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range readLayouts {
		if parsed, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			t.Time = parsed
			return nil
		}
	}
	return fmt.Errorf("unrecognised timestamp %q", s)
}

// MarshalJSON emits Kimai's HTML5 datetime-local format. Kimai interprets
// writes in the user's configured timezone and documents that sending an
// offset produces wrong records, so the offset is deliberately dropped.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Format(kimaiLayout))
}

// Timesheet is a single time entry.
type Timesheet struct {
	ID          int      `json:"id"`
	Begin       Time     `json:"begin"`
	End         Time     `json:"end"`
	Duration    int      `json:"duration"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Billable    bool     `json:"billable"`
	Exported    bool     `json:"exported"`

	// Project and Activity are numeric IDs in collection responses and
	// objects when the request asks for expanded entities. Raw holds
	// whichever form the server sent.
	Project  Ref `json:"project"`
	Activity Ref `json:"activity"`
}

// Running reports whether the entry has no end time.
func (t Timesheet) Running() bool { return t.End.IsZero() }

// Elapsed returns the entry's duration, measuring against now while running.
func (t Timesheet) Elapsed() time.Duration {
	if t.Running() {
		return time.Since(t.Begin.Time).Truncate(time.Second)
	}
	return time.Duration(t.Duration) * time.Second
}

// Ref is an entity reference that Kimai serialises as either a bare ID or a
// full object depending on the endpoint.
type Ref struct {
	ID   int
	Name string
}

// UnmarshalJSON accepts both the bare-ID and expanded-object encodings.
func (r *Ref) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "null" {
		return nil
	}
	if trimmed != "" && trimmed[0] == '{' {
		var obj struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		r.ID, r.Name = obj.ID, obj.Name
		return nil
	}
	return json.Unmarshal(b, &r.ID)
}

// MarshalJSON emits the bare ID, which is what Kimai accepts on writes.
func (r Ref) MarshalJSON() ([]byte, error) { return json.Marshal(r.ID) }

// Project is a Kimai project, owned by a customer.
type Project struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Customer int    `json:"customer"`
	Visible  bool   `json:"visible"`
	Billable bool   `json:"billable"`
}

// Customer is a Kimai customer, the parent of projects.
type Customer struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
}

// Activity is a Kimai activity, either global or bound to one project.
type Activity struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Project *int   `json:"project"`
	Visible bool   `json:"visible"`
}

// Tag is a Kimai tag.
type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// User is a Kimai user account.
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Alias    string `json:"alias"`
	Email    string `json:"email"`
	Language string `json:"language"`
	Timezone string `json:"timezone"`
}
