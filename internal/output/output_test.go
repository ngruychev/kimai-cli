package output

import (
	"github.com/ngruychev/kimai-cli/internal/kimai"
	"testing"
	"time"
	"unicode/utf8"
)

// Durations are shown the way a status bar wants to read them: hours and
// minutes for long spans, dropping to finer units only when there is no hour.
func TestDurationFormatting(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{2*time.Hour + 5*time.Minute, "2h05m"},
		{25 * time.Hour, "25h00m"},
		{-time.Minute, "0s"},
	}
	for _, tc := range tests {
		if got := Duration(tc.in); got != tc.want {
			t.Errorf("Duration(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// truncate must cut on character boundaries: slicing bytes would split a
// multi-byte character and emit invalid UTF-8.
func TestTruncateIsRuneSafe(t *testing.T) {
	truncate := funcs["truncate"].(func(int, string) string)

	tests := []struct {
		n    int
		in   string
		want string
	}{
		{30, "Fix login", "Fix login"},
		{5, "Fix login", "Fix …"},
		{4, "Überprüfung", "Übe…"},
		{3, "日本語のテキスト", "日本…"},
		{1, "Überprüfung", "…"},
		{0, "Überprüfung", "Überprüfung"},
	}
	for _, tc := range tests {
		got := truncate(tc.n, tc.in)
		if got != tc.want {
			t.Errorf("truncate(%d, %q) = %q, want %q", tc.n, tc.in, got, tc.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("truncate(%d, %q) produced invalid UTF-8", tc.n, tc.in)
		}
	}
}

// Times given on the command line are read in this machine's timezone, so
// entries must read back in it too: an entry started at 09:00 should not
// display as 10:00 because the Kimai account uses a different zone.
func TestEntriesDisplayInMachineTimezone(t *testing.T) {
	sofia, err := time.LoadLocation("Europe/Sofia")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// The same instant, as the server would return it: 09:00 local in a
	// UTC+2 zone is 10:00 in Sofia.
	instant := time.Date(2026, 8, 24, 10, 0, 0, 0, sofia)

	entry := NewEntry(kimai.Timesheet{
		Begin: kimai.Time{Time: instant},
		End:   kimai.Time{Time: instant.Add(time.Hour)},
	}, nil)

	begin, err := time.Parse(time.RFC3339, entry.Begin)
	if err != nil {
		t.Fatal(err)
	}
	if !begin.Equal(instant) {
		t.Errorf("begin %s is a different instant from %s", begin, instant)
	}
	_, got := begin.Zone()
	_, want := instant.In(time.Local).Zone()
	if got != want {
		t.Errorf("begin offset = %ds, want %ds (this machine's zone)", got, want)
	}
}
