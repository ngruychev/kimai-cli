package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ngruychev/kimai-cli/internal/kimai"
	"github.com/ngruychev/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

func ptr[T any](v T) *T { return &v }

// parseID accepts a bare ID or a leading-# form as printed by this tool.
func parseID(s string) (int, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if err != nil {
		return 0, fmt.Errorf("not an entry ID: %q", s)
	}
	return id, nil
}

// parseWhen accepts HH:MM (today), RFC3339, or "YYYY-MM-DD HH:MM".
func parseWhen(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	now := time.Now()
	layouts := []struct {
		layout string
		dated  bool
	}{
		{time.RFC3339, true},
		{"2006-01-02 15:04:05", true},
		{"2006-01-02 15:04", true},
		{"2006-01-02T15:04", true},
		{"15:04:05", false},
		{"15:04", false},
	}
	for _, l := range layouts {
		parsed, err := time.ParseInLocation(l.layout, s, time.Local)
		if err != nil {
			continue
		}
		if l.dated {
			return parsed, nil
		}
		return time.Date(now.Year(), now.Month(), now.Day(),
			parsed.Hour(), parsed.Minute(), parsed.Second(), 0, now.Location()), nil
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q: use HH:MM, 'YYYY-MM-DD HH:MM' or RFC3339", s)
}

// parseDay accepts a date, or the words today and yesterday.
func parseDay(s string) (time.Time, error) {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "today":
		return midnight, nil
	case "yesterday":
		return midnight.AddDate(0, 0, -1), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("unrecognised date %q: use YYYY-MM-DD, today or yesterday", s)
	}
	return parsed, nil
}

func promptString(label, initial string) (string, error) {
	var answer string
	err := survey.AskOne(&survey.Input{Message: label, Default: initial}, &answer)
	return answer, err
}

func promptConfirm(label string, initial bool) (bool, error) {
	var answer bool
	err := survey.AskOne(&survey.Confirm{Message: label, Default: initial}, &answer)
	return answer, err
}

// pickProject prompts for a project, labelled by customer.
func pickProject(l *kimai.Lookup) (*kimai.Project, error) {
	if len(l.Projects) == 0 {
		return nil, fmt.Errorf("no projects available")
	}
	labels := make([]string, 0, len(l.Projects))
	for _, p := range l.Projects {
		if customer := l.CustomerName(p.ID); customer != "" {
			labels = append(labels, fmt.Sprintf("%s — %s", customer, p.Name))
			continue
		}
		labels = append(labels, p.Name)
	}

	var choice int
	prompt := &survey.Select{Message: "Project", Options: labels, PageSize: 15}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return nil, err
	}
	return &l.Projects[choice], nil
}

// pickActivity prompts for an activity valid for the given project.
func pickActivity(ctx context.Context, projectID int) (*kimai.Activity, error) {
	activities, err := client.Activities(ctx, projectID, true)
	if err != nil {
		return nil, err
	}
	if len(activities) == 0 {
		return nil, fmt.Errorf("no activities available for this project")
	}
	labels := make([]string, len(activities))
	for i, a := range activities {
		labels[i] = a.Name
	}

	var choice int
	prompt := &survey.Select{Message: "Activity", Options: labels, PageSize: 15}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return nil, err
	}
	return &activities[choice], nil
}

// pickRecentEntry prompts for one of the recent entries, most recent first.
func pickRecentEntry(cmd *cobra.Command, l *kimai.Lookup) (int, error) {
	now := time.Now()
	entries, err := client.Timesheets(cmd.Context(), kimai.TimesheetQuery{
		Begin: now.AddDate(0, 0, -64),
		End:   now.AddDate(0, 0, 1),
		Size:  200,
	})
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("no recent entries to clone")
	}

	// Collapse repeats so the list shows distinct work, newest first.
	seen := map[string]bool{}
	var ids []int
	var labels []string
	for _, t := range entries {
		description := t.Description
		if description == "" {
			description = "(no description)"
		}
		key := fmt.Sprintf("%d|%d|%s", t.Project.ID, t.Activity.ID, description)
		if seen[key] {
			continue
		}
		seen[key] = true
		ids = append(ids, t.ID)
		labels = append(labels, fmt.Sprintf("%s — %s", l.ProjectName(t.Project.ID), description))
	}

	var choice int
	prompt := &survey.Select{Message: "Entry to clone", Options: labels, PageSize: 15}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return 0, err
	}
	return ids[choice], nil
}

// warnDroppedTags reports tags that the server did not store. Kimai silently
// ignores tags that do not already exist when tag creation is disabled, so
// without this the loss is invisible.
func warnDroppedTags(requested []string, entry kimai.Timesheet) {
	if len(requested) == 0 {
		return
	}
	stored := make(map[string]bool, len(entry.Tags))
	for _, tag := range entry.Tags {
		stored[strings.ToLower(tag)] = true
	}
	var dropped []string
	for _, tag := range requested {
		if !stored[strings.ToLower(strings.TrimSpace(tag))] {
			dropped = append(dropped, tag)
		}
	}
	if len(dropped) > 0 {
		fmt.Fprintf(os.Stderr, "warning: tags not stored (they must already exist in Kimai): %s\n",
			strings.Join(dropped, ", "))
	}
}

// pickTags prompts for tags, offering the vocabulary the instance already
// knows. Kimai drops tags that do not exist, so free text is not offered.
func pickTags(ctx context.Context, current []string) ([]string, error) {
	available, err := client.Tags(ctx)
	if err != nil {
		return nil, err
	}
	if len(available) == 0 {
		fmt.Fprintln(os.Stderr, "note: this Kimai instance has no tags defined")
		return nil, nil
	}

	var chosen []string
	prompt := &survey.MultiSelect{
		Message:  "Tags",
		Options:  available,
		Default:  current,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &chosen); err != nil {
		return nil, err
	}
	return chosen, nil
}

// describeEntry renders a one-line summary used when confirming destructive
// actions, so an entry is never deleted by bare ID alone.
func describeEntry(t kimai.Timesheet, l *kimai.Lookup) string {
	description := t.Description
	if description == "" {
		description = "(no description)"
	}
	project := t.Project.Name
	if project == "" && l != nil {
		project = l.ProjectName(t.Project.ID)
	}
	state := output.Duration(t.Elapsed())
	if t.Running() {
		state += ", running"
	}
	return fmt.Sprintf("#%d  %s  [%s]  %s  %s",
		t.ID, t.Begin.Local().Format("2006-01-02 15:04"), state, project, description)
}
