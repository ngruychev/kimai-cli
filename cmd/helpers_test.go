package cmd

import (
	"testing"
	"time"
)

// A bare HH:MM means that time today, which is what a user typing --begin 09:30
// means by it.
func TestParseWhenBareClockTimeMeansToday(t *testing.T) {
	got, err := parseWhen("09:30")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if got.Year() != now.Year() || got.YearDay() != now.YearDay() {
		t.Errorf("parsed to %s, want today", got)
	}
	if got.Hour() != 9 || got.Minute() != 30 {
		t.Errorf("parsed to %s, want 09:30", got)
	}
}

func TestParseWhenAcceptsDatedForms(t *testing.T) {
	for _, in := range []string{"2026-08-24 09:30", "2026-08-24T09:30", "2026-08-24T09:30:00+02:00"} {
		got, err := parseWhen(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got.Day() != 24 || got.Month() != time.August {
			t.Errorf("%s parsed to %s, want 24 August", in, got)
		}
	}
}

func TestParseWhenRejectsNonsense(t *testing.T) {
	if _, err := parseWhen("half past nine"); err == nil {
		t.Error("expected an error for unparseable input")
	}
}

// "today" and "yesterday" are the words a user reaches for; both resolve to
// midnight so they can bound a day range.
func TestParseDayWords(t *testing.T) {
	today, err := parseDay("today")
	if err != nil {
		t.Fatal(err)
	}
	if today.Hour() != 0 || today.Minute() != 0 {
		t.Errorf("today = %s, want midnight", today)
	}

	yesterday, err := parseDay("yesterday")
	if err != nil {
		t.Fatal(err)
	}
	if diff := today.Sub(yesterday); diff != 24*time.Hour {
		t.Errorf("today - yesterday = %s, want 24h", diff)
	}
}

// A two-date range is inclusive of the end date, so `log 2026-08-01 2026-08-01`
// covers all of that one day.
func TestRangeFromArgsIsInclusiveOfEndDate(t *testing.T) {
	begin, end, err := rangeFromArgs([]string{"2026-08-01", "2026-08-01"})
	if err != nil {
		t.Fatal(err)
	}
	if got := end.Sub(begin); got != 24*time.Hour {
		t.Errorf("range spans %s, want 24h", got)
	}
}

func TestRangeFromArgsRejectsBackwardsRange(t *testing.T) {
	if _, _, err := rangeFromArgs([]string{"2026-08-05", "2026-08-01"}); err == nil {
		t.Error("expected an error when the end date precedes the start")
	}
}

// No arguments means today, matching what `log` prints by default.
func TestRangeFromArgsDefaultsToToday(t *testing.T) {
	begin, end, err := rangeFromArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if begin.YearDay() != now.YearDay() {
		t.Errorf("begin = %s, want today", begin)
	}
	if got := end.Sub(begin); got != 24*time.Hour {
		t.Errorf("range spans %s, want 24h", got)
	}
}

// Period flags define whole weeks and months regardless of today's date.
func TestReportRangePeriodFlags(t *testing.T) {
	thisWeekBegin, thisWeekEnd, err := reportRange(nil, true, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := thisWeekEnd.Sub(thisWeekBegin); got != 7*24*time.Hour {
		t.Errorf("this week spans %s, want 168h", got)
	}
	if thisWeekBegin.Weekday() != time.Monday {
		t.Errorf("week starts on %s, want Monday", thisWeekBegin.Weekday())
	}

	monthBegin, monthEnd, err := reportRange(nil, false, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if monthBegin.Day() != 1 {
		t.Errorf("month starts on day %d, want 1", monthBegin.Day())
	}
	if monthEnd.Day() != 1 || !monthEnd.After(monthBegin) {
		t.Errorf("month range %s..%s should end on the next 1st", monthBegin, monthEnd)
	}
}

// Entry IDs are accepted both bare and in the #123 form this tool prints.
func TestParseIDAcceptsPrintedForm(t *testing.T) {
	for _, in := range []string{"123", "#123", " 123 "} {
		got, err := parseID(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != 123 {
			t.Errorf("parseID(%q) = %d, want 123", in, got)
		}
	}
	if _, err := parseID("current"); err == nil {
		t.Error("parseID should reject non-numeric input; 'current' is resolved separately")
	}
}
