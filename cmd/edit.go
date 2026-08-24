package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"os"
	"strings"
	"time"

	"github.com/ngruychev/kimai-cli/internal/kimai"
	"github.com/ngruychev/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	var (
		spec  entrySpec
		out   outputFlags
		begin string
		end   string
	)
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Modify an existing entry",
		Long:  "Updates only the fields given as flags. Use \"current\" for the running entry.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			id, err := resolveEntryID(cmd, args[0])
			if err != nil {
				return err
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}

			var form kimai.TimesheetForm
			touched := false

			if cmd.Flags().Changed("description") {
				form.Description, touched = &spec.description, true
			}
			if cmd.Flags().Changed("project") {
				p, err := l.FindProject(spec.project)
				if err != nil {
					return err
				}
				form.Project, touched = &p.ID, true
			}
			if cmd.Flags().Changed("activity") {
				a, err := l.FindActivity(spec.activity)
				if err != nil {
					return err
				}
				form.Activity, touched = &a.ID, true
			}
			if cmd.Flags().Changed("tags") {
				form.Tags, touched = ptr(strings.Join(spec.tags, ",")), true
			}
			if begin != "" {
				when, err := parseWhen(begin)
				if err != nil {
					return err
				}
				form.Begin, touched = &kimai.Time{Time: when}, true
			}
			if end != "" {
				when, err := parseWhen(end)
				if err != nil {
					return err
				}
				form.End, touched = &kimai.Time{Time: when}, true
			}

			if !touched {
				if !interactive {
					return fmt.Errorf("nothing to change: pass a flag, or use --interactive")
				}
				current, err := client.Timesheet(cmd.Context(), id)
				if err != nil {
					return err
				}
				if err := promptEdits(cmd, l, current, &form); err != nil {
					return err
				}
			}

			updated, err := client.UpdateTimesheet(cmd.Context(), id, form)
			if err != nil {
				return err
			}
			warnDroppedTags(spec.tags, *updated)
			return out.renderEntry(output.NewEntry(*updated, l))
		},
	}
	spec.register(cmd)
	out.register(cmd)
	cmd.Flags().StringVar(&begin, "begin", "", "new start time (HH:MM or RFC3339)")
	cmd.Flags().StringVar(&end, "end", "", "new end time (HH:MM or RFC3339)")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "delete <id>...",
		Aliases: []string{"rm"},
		Short:   "Delete entries",
		Long:    "Deletes entries by ID. Use \"current\" for the running entry.",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}

			// Resolve each entry up front so the user sees what is at stake.
			entries := make([]kimai.Timesheet, 0, len(args))
			for _, arg := range args {
				id, err := resolveEntryID(cmd, arg)
				if err != nil {
					return err
				}
				entry, err := client.Timesheet(cmd.Context(), id)
				if err != nil {
					return err
				}
				entries = append(entries, *entry)
			}

			if !force {
				// Preview only when asking; --force reports each deletion below.
				for _, entry := range entries {
					fmt.Fprintf(os.Stderr, "  %s\n", describeEntry(entry, l))
				}
				noun := "entries"
				if len(entries) == 1 {
					noun = "entry"
				}
				ok, err := promptConfirm(fmt.Sprintf("Delete the %d %s above?", len(entries), noun), false)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("aborted")
				}
			}
			for _, entry := range entries {
				if err := client.DeleteTimesheet(cmd.Context(), entry.ID); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "deleted %s\n", describeEntry(entry, l))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}

func newManualCmd() *cobra.Command {
	var (
		spec     entrySpec
		out      outputFlags
		begin    string
		end      string
		duration string
	)
	cmd := &cobra.Command{
		Use:   "manual",
		Short: "Create a complete, already-finished entry",
		Long:  "Creates an entry with both a start and an end, without starting a timer.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			if begin == "" {
				return fmt.Errorf("--begin is required")
			}
			if end == "" && duration == "" {
				return fmt.Errorf("give --end or --duration")
			}

			start, err := parseWhen(begin)
			if err != nil {
				return err
			}
			var stop time.Time
			if end != "" {
				if stop, err = parseWhen(end); err != nil {
					return err
				}
			} else {
				d, err := time.ParseDuration(duration)
				if err != nil {
					return fmt.Errorf("bad --duration %q: use forms like 90m or 1h30m", duration)
				}
				stop = start.Add(d)
			}
			if !stop.After(start) {
				return fmt.Errorf("end is not after begin")
			}

			projectID, activityID, err := spec.resolve(cmd)
			if err != nil {
				return err
			}
			form := kimai.TimesheetForm{
				Begin:       &kimai.Time{Time: start},
				End:         &kimai.Time{Time: stop},
				Project:     &projectID,
				Activity:    &activityID,
				Description: &spec.description,
			}
			if len(spec.tags) > 0 {
				form.Tags = ptr(strings.Join(spec.tags, ","))
			}

			created, err := client.CreateTimesheet(cmd.Context(), form)
			if err != nil {
				return err
			}
			warnDroppedTags(spec.tags, *created)
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}
			return out.renderEntry(output.NewEntry(*created, l))
		},
	}
	spec.register(cmd)
	out.register(cmd)
	cmd.Flags().StringVar(&begin, "begin", "", "start time (required)")
	cmd.Flags().StringVar(&end, "end", "", "end time")
	cmd.Flags().StringVar(&duration, "duration", "", "length instead of --end, e.g. 90m")
	return cmd
}

// promptEdits asks which fields to change and prompts for each, so that
// interactive editing is not limited to the description.
func promptEdits(cmd *cobra.Command, l *kimai.Lookup, current *kimai.Timesheet, form *kimai.TimesheetForm) error {
	ctx := cmd.Context()

	const (
		fDescription = "description"
		fProject     = "project"
		fActivity    = "activity"
		fTags        = "tags"
		fBegin       = "begin"
		fEnd         = "end"
	)
	options := []string{fDescription, fProject, fActivity, fTags, fBegin}
	if !current.Running() {
		options = append(options, fEnd)
	}

	var fields []string
	prompt := &survey.MultiSelect{Message: "Fields to edit", Options: options}
	if err := survey.AskOne(prompt, &fields); err != nil {
		return err
	}
	if len(fields) == 0 {
		return fmt.Errorf("nothing selected")
	}

	chosen := make(map[string]bool, len(fields))
	for _, f := range fields {
		chosen[f] = true
	}

	if chosen[fDescription] {
		description, err := promptString("Description", current.Description)
		if err != nil {
			return err
		}
		form.Description = &description
	}
	if chosen[fProject] {
		project, err := pickProject(l)
		if err != nil {
			return err
		}
		form.Project = &project.ID
	}
	if chosen[fActivity] {
		projectID := current.Project.ID
		if form.Project != nil {
			projectID = *form.Project
		}
		activity, err := pickActivity(ctx, projectID)
		if err != nil {
			return err
		}
		form.Activity = &activity.ID
	}
	if chosen[fTags] {
		tags, err := pickTags(ctx, current.Tags)
		if err != nil {
			return err
		}
		form.Tags = ptr(strings.Join(tags, ","))
	}
	if chosen[fBegin] {
		answer, err := promptString("Begin (HH:MM or YYYY-MM-DD HH:MM)", current.Begin.Local().Format("15:04"))
		if err != nil {
			return err
		}
		when, err := parseWhen(answer)
		if err != nil {
			return err
		}
		form.Begin = &kimai.Time{Time: when}
	}
	if chosen[fEnd] {
		answer, err := promptString("End (HH:MM or YYYY-MM-DD HH:MM)", current.End.Local().Format("15:04"))
		if err != nil {
			return err
		}
		when, err := parseWhen(answer)
		if err != nil {
			return err
		}
		form.End = &kimai.Time{Time: when}
	}
	return nil
}
