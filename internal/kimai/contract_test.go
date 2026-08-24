package kimai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// capture records what the client sent, filled in once the request arrives.
type capture struct {
	Method string
	Query  url.Values
	Body   string
}

// captureRequest serves one canned response and records what was sent.
func captureRequest(t *testing.T, response string) (*Client, *capture) {
	t.Helper()
	got := &capture{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.Method = r.Method
		got.Query = r.URL.Query()
		got.Body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "token"), got
}

// Kimai stores writes in the user's own timezone and documents that sending an
// offset produces wrong records, so begin and end must go out as bare local
// datetimes.
func TestWriteTimestampsCarryNoTimezoneOffset(t *testing.T) {
	client, got := captureRequest(t, `{"id": 1}`)

	begin := time.Date(2026, 8, 24, 9, 30, 0, 0, time.FixedZone("CEST", 2*3600))
	_, err := client.CreateTimesheet(context.Background(), TimesheetForm{
		Begin: &Time{Time: begin},
	})
	if err != nil {
		t.Fatal(err)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(got.Body), &sent); err != nil {
		t.Fatalf("request body was not JSON: %q", got.Body)
	}
	began, _ := sent["begin"].(string)
	if began != "2026-08-24T09:30:00" {
		t.Errorf("begin = %q, want %q (no offset)", began, "2026-08-24T09:30:00")
	}
	if strings.ContainsAny(began, "+Z") {
		t.Errorf("begin %q carries a timezone; Kimai records it wrongly", began)
	}
}

// Kimai declares restart's copy as a RequestParam, which it reads from the
// request body. Sent as a query parameter it is ignored and the new entry
// loses its description, tags and billable state.
func TestRestartSendsCopyInTheRequestBody(t *testing.T) {
	client, got := captureRequest(t, `{"id": 2}`)

	if _, err := client.RestartTimesheet(context.Background(), 7); err != nil {
		t.Fatal(err)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(got.Body), &sent); err != nil {
		t.Fatalf("request body was not JSON: %q", got.Body)
	}
	if sent["copy"] != "all" {
		t.Errorf("body copy = %v, want \"all\"", sent["copy"])
	}
	if stray := got.Query.Get("copy"); stray != "" {
		t.Errorf("copy was also sent as query %q; Kimai ignores it there", stray)
	}
	if got.Method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", got.Method)
	}
}

// A range wider than one page must return every entry, not just the first
// page, or reports and weekly totals quietly understate the time worked.
func TestTimesheetsFollowsPagination(t *testing.T) {
	const perPage, pages = 2, 3
	var requested []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		requested = append(requested, r.URL.Query().Get("page"))

		batch := make([]Timesheet, 0, perPage)
		for i := range perPage {
			batch = append(batch, Timesheet{ID: (page-1)*perPage + i + 1})
		}
		w.Header().Set("X-Total-Pages", strconv.Itoa(pages))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(batch)
	}))
	defer srv.Close()

	client := New(srv.URL, "token")
	entries, err := client.Timesheets(context.Background(), TimesheetQuery{Size: perPage})
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != perPage*pages {
		t.Errorf("got %d entries across %d pages, want %d", len(entries), len(requested), perPage*pages)
	}
	if len(requested) != pages {
		t.Errorf("requested pages %v, want %d requests", requested, pages)
	}
	for i, e := range entries {
		if e.ID != i+1 {
			t.Errorf("entry %d has ID %d, want %d: pages out of order", i, e.ID, i+1)
		}
	}
}

// Asking for one specific page must not trigger the follow-all loop.
func TestExplicitPageFetchesOnlyThatPage(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("X-Total-Pages", "5")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Timesheet{{ID: 1}})
	}))
	defer srv.Close()

	client := New(srv.URL, "token")
	if _, err := client.Timesheets(context.Background(), TimesheetQuery{Page: 2}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("made %d requests for an explicit page, want 1", calls)
	}
}

// An instance that omits pagination headers must still terminate, returning
// what it did send rather than looping.
func TestPaginationTerminatesWithoutHeaders(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Timesheet{{ID: calls}})
	}))
	defer srv.Close()

	client := New(srv.URL, "token")
	entries, err := client.Timesheets(context.Background(), TimesheetQuery{Size: 500})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("made %d requests with no pagination headers, want 1", calls)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}

// Activity names are only unique within a project, so resolution must be
// scoped to the project's own candidates.
func TestMatchActivityIsScopedToItsCandidates(t *testing.T) {
	website := []Activity{{ID: 1, Name: "Development"}, {ID: 2, Name: "Review"}}

	got, err := MatchActivity(website, "Development")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 1 {
		t.Errorf("resolved to activity %d, want 1", got.ID)
	}

	// An activity belonging to another project must not resolve here.
	if _, err := MatchActivity(website, "Deployment"); err == nil {
		t.Error("expected an error for an activity outside the project")
	}

	// An ID from another project must not resolve either.
	if _, err := MatchActivity(website, "99"); err == nil {
		t.Error("expected an error for an ID outside the project")
	}
}

// A term matching several activities is ambiguous and must not silently pick one.
func TestMatchActivityRejectsAmbiguousTerms(t *testing.T) {
	candidates := []Activity{{ID: 1, Name: "Backend development"}, {ID: 2, Name: "Frontend development"}}
	_, err := MatchActivity(candidates, "development")
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	if !strings.Contains(err.Error(), "Backend development") ||
		!strings.Contains(err.Error(), "Frontend development") {
		t.Errorf("error %q should name both candidates", err)
	}
}

// Kimai reads a write's naive timestamp as wall-clock time in the account's
// own timezone. When that differs from this machine's, the value must be
// converted or the entry lands at the wrong moment.
func TestWritesUseTheAccountTimezoneNotTheMachines(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/users/me" {
			fmt.Fprint(w, `{"id":1,"username":"u","timezone":"Europe/Sofia"}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		fmt.Fprint(w, `{"id":1}`)
	}))
	defer srv.Close()

	amsterdam, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// 14:32 in Amsterdam is the same instant as 15:32 in Sofia.
	begin := time.Date(2026, 8, 24, 14, 32, 0, 0, amsterdam)

	client := New(srv.URL, "token")
	if _, err := client.CreateTimesheet(context.Background(), TimesheetForm{Begin: &Time{Time: begin}}); err != nil {
		t.Fatal(err)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body was not JSON: %q", body)
	}
	if got := sent["begin"]; got != "2026-08-24T15:32:00" {
		t.Errorf("begin = %v, want 2026-08-24T15:32:00 (the instant expressed in Sofia)", got)
	}
}
