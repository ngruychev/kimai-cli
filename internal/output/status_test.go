package output_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ngruychev/kimai-cli/internal/kimai"
	"github.com/ngruychev/kimai-cli/internal/output"
)

// newCountingKimai serves empty timesheet listings and counts how many
// listing requests were made, so tests can assert on API traffic.
func newCountingKimai(t *testing.T, calls *int32) *kimai.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/timesheets") {
			atomic.AddInt32(calls, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]kimai.Timesheet{})
	}))
	t.Cleanup(srv.Close)
	return kimai.New(srv.URL, "token")
}

// A template that names no aggregate must not cost an API call: this is the
// guarantee that makes `status --format` cheap enough for a status bar.
func TestTemplateWithoutAggregatesMakesNoRequests(t *testing.T) {
	var calls int32
	client := newCountingKimai(t, &calls)
	entry := kimai.Timesheet{Description: "writing docs", Duration: 3900,
		End: kimai.Time{Time: time.Now()}}
	status := output.NewStatus(context.Background(), client, nil, &entry)

	var buf strings.Builder
	if err := output.Template(&buf, "{{.Description}} {{.CurrentDuration}}", status); err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(buf.String()); got != "writing docs 1h05m" {
		t.Errorf("rendered %q, want %q", got, "writing docs 1h05m")
	}
	if calls != 0 {
		t.Errorf("made %d timesheet requests, want 0", calls)
	}
}

// Naming a daily total costs exactly one request, and naming it twice still
// costs one: the result is memoised.
func TestDailyAggregateIsFetchedOnceAndOnlyWhenNamed(t *testing.T) {
	var calls int32
	client := newCountingKimai(t, &calls)
	status := output.NewStatus(context.Background(), client, nil, nil)

	var buf strings.Builder
	if err := output.Template(&buf, "{{.DailyDuration}} {{.DailyDuration}}", status); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("made %d timesheet requests, want 1", calls)
	}
}

// Daily and weekly are independent windows, so naming both costs two requests.
func TestDailyAndWeeklyAreSeparateRequests(t *testing.T) {
	var calls int32
	client := newCountingKimai(t, &calls)
	status := output.NewStatus(context.Background(), client, nil, nil)

	var buf strings.Builder
	if err := output.Template(&buf, "{{.DailyDuration}}/{{.WeeklyDuration}}", status); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("made %d timesheet requests, want 2", calls)
	}
}

// --json is a complete snapshot, so it resolves the aggregates even though
// --format leaves them lazy.
func TestJSONSnapshotResolvesAggregates(t *testing.T) {
	var calls int32
	client := newCountingKimai(t, &calls)
	status := output.NewStatus(context.Background(), client, nil, nil)

	var buf strings.Builder
	if err := output.JSON(&buf, status); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"running", "current_duration", "daily_duration", "weekly_duration"} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("JSON output is missing %q", field)
		}
	}
	if decoded["running"] != false {
		t.Errorf("running = %v with no active entry, want false", decoded["running"])
	}
}

// With the clock stopped, a status bar template must still render rather than
// erroring or printing a nil.
func TestStoppedClockRendersEmptyFields(t *testing.T) {
	var calls int32
	client := newCountingKimai(t, &calls)
	status := output.NewStatus(context.Background(), client, nil, nil)

	var buf strings.Builder
	if err := output.Template(&buf, "[{{.Description}}]", status); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("rendered %q, want %q", got, "[]")
	}
}

// Resolving entity names costs several API calls, so a status bar template
// that shows only a duration must not pay for them.
func TestNamesAreResolvedOnlyWhenTheTemplateAsks(t *testing.T) {
	entry := kimai.Timesheet{Project: kimai.Ref{ID: 5}, Description: "work"}

	var lookups int
	lookup := func(context.Context) (*kimai.Lookup, error) {
		lookups++
		return nil, nil
	}

	var calls int32
	client := newCountingKimai(t, &calls)

	status := output.NewStatus(context.Background(), client, lookup, &entry)
	var buf strings.Builder
	if err := output.Template(&buf, "{{.Description}} {{.CurrentDuration}}", status); err != nil {
		t.Fatal(err)
	}
	if lookups != 0 {
		t.Errorf("resolved names %d times for a template that names none, want 0", lookups)
	}

	// Naming a project resolves the index once, and only once.
	status2 := output.NewStatus(context.Background(), client, lookup, &entry)
	buf.Reset()
	if err := output.Template(&buf, "{{.Project}} {{.Customer}} {{.Activity}}", status2); err != nil {
		t.Fatal(err)
	}
	if lookups != 1 {
		t.Errorf("resolved names %d times for three name fields, want 1", lookups)
	}
}
