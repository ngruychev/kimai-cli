package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ngruychev/kimai-cli/internal/kimai"
	"github.com/ngruychev/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

// entrySpec collects the flags shared by the commands that create entries.
type entrySpec struct {
	project     string
	activity    string
	description string
	tags        []string
}

func (s *entrySpec) register(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&s.project, "project", "p", "", "project name or ID")
	cmd.Flags().StringVarP(&s.activity, "activity", "a", "", "activity name or ID")
	cmd.Flags().StringVarP(&s.description, "description", "d", "", "entry description")
	cmd.Flags().StringSliceVar(&s.tags, "tags", nil, "comma-separated tags")
}

// resolve turns the spec's names into IDs, prompting when interactive.
func (s *entrySpec) resolve(cmd *cobra.Command) (projectID, activityID int, err error) {
	ctx := cmd.Context()
	l, err := lookup(ctx)
	if err != nil {
		return 0, 0, err
	}

	switch {
	case s.project != "":
		p, err := l.FindProject(s.project)
		if err != nil {
			return 0, 0, err
		}
		projectID = p.ID
	case interactive:
		p, err := pickProject(l)
		if err != nil {
			return 0, 0, err
		}
		projectID = p.ID
	case cfg.DefaultProject > 0:
		projectID = cfg.DefaultProject
	default:
		return 0, 0, fmt.Errorf("no project: pass --project, use --interactive, or set default_project")
	}

	switch {
	case s.activity != "":
		// Resolve against the activities valid for this project, so a name
		// shared with another project cannot be picked by mistake.
		candidates, err := client.Activities(ctx, projectID, true)
		if err != nil {
			return 0, 0, err
		}
		a, err := kimai.MatchActivity(candidates, s.activity)
		if err != nil {
			return 0, 0, err
		}
		activityID = a.ID
	case interactive:
		a, err := pickActivity(ctx, projectID)
		if err != nil {
			return 0, 0, err
		}
		activityID = a.ID
	case cfg.DefaultActivity > 0:
		activityID = cfg.DefaultActivity
	default:
		return 0, 0, fmt.Errorf("no activity: pass --activity, use --interactive, or set default_activity")
	}
	return projectID, activityID, nil
}

func newInCmd() *cobra.Command {
	var (
		spec  entrySpec
		out   outputFlags
		begin string
	)
	cmd := &cobra.Command{
		Use:     "in",
		Aliases: []string{"start"},
		Short:   "Start a new time entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			projectID, activityID, err := spec.resolve(cmd)
			if err != nil {
				return err
			}
			if interactive {
				if spec.description == "" {
					if spec.description, err = promptString("Description", ""); err != nil {
						return err
					}
				}
				if !cmd.Flags().Changed("tags") {
					if spec.tags, err = pickTags(cmd.Context(), nil); err != nil {
						return err
					}
				}
			}

			start := time.Now()
			if begin != "" {
				if start, err = parseWhen(begin); err != nil {
					return err
				}
			}

			form := kimai.TimesheetForm{
				Begin:       &kimai.Time{Time: start},
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
	cmd.Flags().StringVar(&begin, "begin", "", "start time (HH:MM or RFC3339), defaults to now")
	return cmd
}

func newOutCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:     "out",
		Aliases: []string{"stop"},
		Short:   "Stop the running time entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			stopped, err := client.StopAll(cmd.Context())
			if err != nil {
				return err
			}
			if len(stopped) == 0 {
				fmt.Fprintln(os.Stderr, "no running entry")
				return nil
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}
			return out.renderEntries(output.NewEntries(stopped, l), false)
		},
	}
	out.register(cmd)
	return cmd
}

func newCloneCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:   "clone [id]",
		Short: "Copy a previous entry and start it running",
		Long: "Restarts a previous entry, carrying over its project, activity,\n" +
			"description, tags and billable flag. With no ID and --interactive,\n" +
			"pick from recent entries.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}

			var id int
			switch {
			case len(args) == 1:
				if id, err = parseID(args[0]); err != nil {
					return err
				}
			case interactive:
				if id, err = pickRecentEntry(cmd, l); err != nil {
					return err
				}
			default:
				return fmt.Errorf("no entry given: pass an ID or use --interactive")
			}

			started, err := client.RestartTimesheet(cmd.Context(), id)
			if err != nil {
				return err
			}
			if interactive {
				if started, err = amendClone(cmd, started); err != nil {
					return err
				}
			}
			return out.renderEntry(output.NewEntry(*started, l))
		},
	}
	out.register(cmd)
	return cmd
}

// amendClone offers to adjust the freshly started copy. A cloned entry
// usually needs a tweak to its description, and doing it here avoids a
// separate edit against an already-running timer.
func amendClone(cmd *cobra.Command, started *kimai.Timesheet) (*kimai.Timesheet, error) {
	change, err := promptConfirm("Adjust description or tags?", false)
	if err != nil || !change {
		return started, err
	}

	description, err := promptString("Description", started.Description)
	if err != nil {
		return nil, err
	}
	tags, err := pickTags(cmd.Context(), started.Tags)
	if err != nil {
		return nil, err
	}

	form := kimai.TimesheetForm{
		Description: &description,
		Tags:        ptr(strings.Join(tags, ",")),
	}
	updated, err := client.UpdateTimesheet(cmd.Context(), started.ID, form)
	if err != nil {
		return nil, err
	}
	warnDroppedTags(tags, *updated)
	return updated, nil
}
