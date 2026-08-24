package kimai

import (
	"encoding/json"
	"testing"
	"time"
)

// Kimai serialises project and activity as a bare ID in collection responses
// and as an object in expanded ones. Both must decode.
func TestRefAcceptsBothServerEncodings(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantID   int
		wantName string
	}{
		{"bare id", `{"project": 12}`, 12, ""},
		{"expanded object", `{"project": {"id": 12, "name": "Website"}}`, 12, "Website"},
		{"null", `{"project": null}`, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var entry Timesheet
			if err := json.Unmarshal([]byte(tc.payload), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Project.ID != tc.wantID {
				t.Errorf("ID = %d, want %d", entry.Project.ID, tc.wantID)
			}
			if entry.Project.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", entry.Project.Name, tc.wantName)
			}
		})
	}
}

// Writes must go back as a bare ID regardless of how the value was read.
func TestRefWritesBackAsBareID(t *testing.T) {
	encoded, err := json.Marshal(Ref{ID: 7, Name: "Website"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "7" {
		t.Errorf("marshalled to %s, want 7", encoded)
	}
}

// Kimai timestamps arrive with and without a zone offset depending on endpoint.
func TestTimeAcceptsBothTimestampForms(t *testing.T) {
	for _, payload := range []string{`"2026-08-24T09:30:00+02:00"`, `"2026-08-24T09:30:00"`} {
		var parsed Time
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			t.Fatalf("%s: %v", payload, err)
		}
		if parsed.Hour() != 9 || parsed.Minute() != 30 {
			t.Errorf("%s parsed to %s, want 09:30", payload, parsed.Format(time.RFC3339))
		}
	}
}

// A running entry has no end time, and its elapsed time is measured to now
// rather than read from the stored duration.
func TestRunningEntryMeasuresElapsedToNow(t *testing.T) {
	entry := Timesheet{
		Begin:    Time{Time: time.Now().Add(-90 * time.Minute)},
		Duration: 0,
	}
	if !entry.Running() {
		t.Fatal("entry with no end time should report as running")
	}
	elapsed := entry.Elapsed()
	if elapsed < 89*time.Minute || elapsed > 91*time.Minute {
		t.Errorf("elapsed = %s, want about 90m", elapsed)
	}
}

// A stopped entry reports the duration the server recorded.
func TestStoppedEntryUsesStoredDuration(t *testing.T) {
	entry := Timesheet{
		Begin:    Time{Time: time.Now().Add(-3 * time.Hour)},
		End:      Time{Time: time.Now()},
		Duration: 3600,
	}
	if entry.Running() {
		t.Fatal("entry with an end time should not report as running")
	}
	if got := entry.Elapsed(); got != time.Hour {
		t.Errorf("elapsed = %s, want 1h", got)
	}
}

// Kimai 2.58 returns numeric offsets without a colon, which RFC3339 rejects.
func TestTimeAcceptsColonlessOffset(t *testing.T) {
	var parsed Time
	if err := json.Unmarshal([]byte(`"2026-08-24T14:38:00+0300"`), &parsed); err != nil {
		t.Fatalf("failed to parse a real Kimai timestamp: %v", err)
	}
	if parsed.Hour() != 14 || parsed.Minute() != 38 {
		t.Errorf("parsed to %s, want 14:38", parsed.Format(time.RFC3339))
	}
	if _, offset := parsed.Zone(); offset != 3*3600 {
		t.Errorf("offset = %ds, want 10800 (+03:00)", offset)
	}
}
