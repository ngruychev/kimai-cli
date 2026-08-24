// Package output renders entries as JSON, Go templates, or aligned tables.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/ngruychev/kimai-cli/internal/kimai"
)

// Entry is the rendered form of a timesheet: names resolved, durations
// pre-formatted. This is the stable contract for --json and --format.
type Entry struct {
	ID          int      `json:"id"`
	Description string   `json:"description"`
	Project     string   `json:"project"`
	ProjectID   int      `json:"project_id"`
	Customer    string   `json:"customer"`
	Activity    string   `json:"activity"`
	ActivityID  int      `json:"activity_id"`
	Begin       string   `json:"begin"`
	End         string   `json:"end"`
	Duration    string   `json:"duration"`
	Seconds     int      `json:"seconds"`
	Tags        []string `json:"tags"`
	Billable    bool     `json:"billable"`
	Exported    bool     `json:"exported"`
	Running     bool     `json:"running"`
}

// NewEntry renders a timesheet using lookup to resolve entity names.
//
// Times are presented in this machine's timezone, matching the timezone in
// which times given on the command line are interpreted, so that an entry
// started at 09:00 also reads back as 09:00.
func NewEntry(t kimai.Timesheet, lookup *kimai.Lookup) Entry {
	projectID, activityID := t.Project.ID, t.Activity.ID
	project, customer, activity := t.Project.Name, "", t.Activity.Name
	if lookup != nil {
		if project == "" {
			project = lookup.ProjectName(projectID)
		}
		if activity == "" {
			activity = lookup.ActivityName(activityID)
		}
		customer = lookup.CustomerName(projectID)
	}

	elapsed := t.Elapsed()
	e := Entry{
		ID:          t.ID,
		Description: t.Description,
		Project:     project,
		ProjectID:   projectID,
		Customer:    customer,
		Activity:    activity,
		ActivityID:  activityID,
		Begin:       t.Begin.Local().Format(time.RFC3339),
		Duration:    Duration(elapsed),
		Seconds:     int(elapsed.Seconds()),
		Tags:        t.Tags,
		Billable:    t.Billable,
		Exported:    t.Exported,
		Running:     t.Running(),
	}
	if e.Tags == nil {
		e.Tags = []string{}
	}
	if !t.End.IsZero() {
		e.End = t.End.Local().Format(time.RFC3339)
	}
	return e
}

// NewEntries renders a slice of timesheets.
func NewEntries(ts []kimai.Timesheet, lookup *kimai.Lookup) []Entry {
	out := make([]Entry, 0, len(ts))
	for _, t := range ts {
		out = append(out, NewEntry(t, lookup))
	}
	return out
}

// Duration formats a duration as Kimai displays it, e.g. "2h05m" or "48s".
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	hours, minutes, seconds := total/3600, (total%3600)/60, total%60
	switch {
	case hours > 0:
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// JSON writes v as indented JSON.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Template renders v through a Go text/template. Methods on v are invoked
// only when the template references them, so callers can expose expensive
// fields lazily.
func Template(w io.Writer, tmpl string, v any) error {
	t, err := template.New("format").Funcs(funcs).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("bad --format template: %w", err)
	}
	if err := t.Execute(w, v); err != nil {
		return err
	}
	if !strings.HasSuffix(tmpl, "\n") {
		fmt.Fprintln(w)
	}
	return nil
}

var funcs = template.FuncMap{
	"truncate": func(n int, s string) string {
		runes := []rune(s)
		if n <= 0 || len(runes) <= n {
			return s
		}
		if n == 1 {
			return "…"
		}
		return string(runes[:n-1]) + "…"
	},
	"join":  strings.Join,
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"default": func(fallback, s string) string {
		if strings.TrimSpace(s) == "" {
			return fallback
		}
		return s
	},
}

// Table writes entries as an aligned, human-readable table.
func Table(w io.Writer, entries []Entry, showDate bool) error {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No entries")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "ID\tSTART\tDURATION\tPROJECT\tACTIVITY\tDESCRIPTION"
	if showDate {
		header = "ID\tDATE\tSTART\tDURATION\tPROJECT\tACTIVITY\tDESCRIPTION"
	}
	fmt.Fprintln(tw, header)

	var total int
	for _, e := range entries {
		total += e.Seconds
		begin, _ := time.Parse(time.RFC3339, e.Begin)
		duration := e.Duration
		if e.Running {
			duration += " *"
		}
		description := e.Description
		if description == "" {
			description = "-"
		}
		if showDate {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", e.ID,
				begin.Format("2006-01-02"), begin.Format("15:04"),
				duration, e.Project, e.Activity, description)
		} else {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", e.ID,
				begin.Format("15:04"), duration, e.Project, e.Activity, description)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	noun := "entries"
	if len(entries) == 1 {
		noun = "entry"
	}
	fmt.Fprintf(w, "\nTotal: %s across %d %s\n",
		Duration(time.Duration(total)*time.Second), len(entries), noun)
	return nil
}

// EntryFields lists the template fields available on an entry, derived from
// the struct itself so the help text cannot drift from the code.
func EntryFields() []string {
	t := reflect.TypeOf(Entry{})
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		names = append(names, "."+t.Field(i).Name)
	}
	return names
}
