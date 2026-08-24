package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/ngruychev/kimai-cli/internal/kimai"
	"github.com/ngruychev/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the running entry",
		Long: "Prints the running entry.\n\n" + formatHelp(output.StatusFields()) +
			"\n\nProject, Customer and Activity resolve the entity index, and the\n" +
			"daily and weekly totals each cost one extra API call. All are\n" +
			"fetched only when the template names them.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			ctx := cmd.Context()
			running, err := client.Active(ctx)
			if err != nil {
				return err
			}

			var active *kimai.Timesheet
			if len(running) > 0 {
				active = &running[0]
			}

			format := out.format
			if format == "" && !out.json && !out.quiet && cfg.StatusFormat != "" {
				format = cfg.StatusFormat
			}
			// lookup is passed unresolved: names cost extra calls, so they are
			// fetched only if the chosen output actually needs them.
			status := output.NewStatus(ctx, client, lookup, active)

			switch {
			case out.json:
				return output.JSON(os.Stdout, status)
			case out.quiet:
				fmt.Println(status.ID())
				return nil
			case format != "":
				return output.Template(os.Stdout, format, status)
			case active == nil:
				fmt.Fprintln(os.Stderr, "no running entry")
				return nil
			default:
				l, err := lookup(ctx)
				if err != nil {
					return err
				}
				printEntry(output.NewEntry(*active, l))
				return nil
			}
		},
	}
	out.register(cmd)
	return cmd
}

func newLogCmd() *cobra.Command {
	var (
		out      outputFlags
		project  string
		activity string
		all      bool
	)
	cmd := &cobra.Command{
		Use:   "log [start] [end]",
		Short: "List entries for a day or date range",
		Long: "With no arguments, lists today. With one argument, lists that day.\n" +
			"With two, lists the inclusive range. Dates are YYYY-MM-DD, or the\n" +
			"words today and yesterday.\n\n" + formatHelp(output.EntryFields()),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			begin, end, err := rangeFromArgs(args)
			if err != nil {
				return err
			}
			return listEntries(cmd, out, begin, end, project, activity, all)
		},
	}
	out.register(cmd)
	cmd.Flags().StringVarP(&project, "project", "p", "", "filter by project name or ID")
	cmd.Flags().StringVarP(&activity, "activity", "a", "", "filter by activity name or ID")
	cmd.Flags().BoolVar(&all, "all-users", false, "include other users' entries")
	return cmd
}

func newReportCmd() *cobra.Command {
	var (
		out       outputFlags
		project   string
		activity  string
		all       bool
		lastWeek  bool
		thisWeek  bool
		lastMonth bool
		thisMonth bool
	)
	cmd := &cobra.Command{
		Use:   "report [start] [end]",
		Short: "Report entries over a date range",
		Long:  "Reports entries over a date range.\n\n" + formatHelp(output.EntryFields()),
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			begin, end, err := reportRange(args, thisWeek, lastWeek, thisMonth, lastMonth)
			if err != nil {
				return err
			}
			return listEntries(cmd, out, begin, end, project, activity, all)
		},
	}
	out.register(cmd)
	cmd.Flags().StringVarP(&project, "project", "p", "", "filter by project name or ID")
	cmd.Flags().StringVarP(&activity, "activity", "a", "", "filter by activity name or ID")
	cmd.Flags().BoolVar(&all, "all-users", false, "include other users' entries")
	cmd.Flags().BoolVar(&thisWeek, "this-week", false, "report the current week")
	cmd.Flags().BoolVar(&lastWeek, "last-week", false, "report the previous week")
	cmd.Flags().BoolVar(&thisMonth, "this-month", false, "report the current month")
	cmd.Flags().BoolVar(&lastMonth, "last-month", false, "report the previous month")
	return cmd
}

// listEntries queries and renders a date range, shared by log and report.
func listEntries(cmd *cobra.Command, out outputFlags, begin, end time.Time,
	project, activity string, allUsers bool) error {

	ctx := cmd.Context()
	l, err := lookup(ctx)
	if err != nil {
		return err
	}

	q := kimai.TimesheetQuery{Begin: begin, End: end}
	if allUsers {
		q.Users = "all"
	}
	if project != "" {
		p, err := l.FindProject(project)
		if err != nil {
			return err
		}
		q.Project = p.ID
	}
	if activity != "" {
		a, err := l.FindActivity(activity)
		if err != nil {
			return err
		}
		q.Activity = a.ID
	}

	entries, err := client.Timesheets(ctx, q)
	if err != nil {
		return err
	}
	// Kimai returns newest first; chronological reads better in a log.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	spansDays := end.Sub(begin) > 24*time.Hour
	return out.renderEntries(output.NewEntries(entries, l), spansDays)
}

// rangeFromArgs turns zero, one or two date arguments into a half-open range.
func rangeFromArgs(args []string) (time.Time, time.Time, error) {
	switch len(args) {
	case 0:
		start, err := parseDay("today")
		return start, start.AddDate(0, 0, 1), err
	case 1:
		start, err := parseDay(args[0])
		return start, start.AddDate(0, 0, 1), err
	default:
		start, err := parseDay(args[0])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		stop, err := parseDay(args[1])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if stop.Before(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("end date is before start date")
		}
		return start, stop.AddDate(0, 0, 1), nil
	}
}

// reportRange applies the shorthand period flags, falling back to arguments.
func reportRange(args []string, thisWeek, lastWeek, thisMonth, lastMonth bool) (time.Time, time.Time, error) {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monday := midnight.AddDate(0, 0, -((int(now.Weekday()) + 6) % 7))
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	switch {
	case thisWeek:
		return monday, monday.AddDate(0, 0, 7), nil
	case lastWeek:
		return monday.AddDate(0, 0, -7), monday, nil
	case thisMonth:
		return firstOfMonth, firstOfMonth.AddDate(0, 1, 0), nil
	case lastMonth:
		return firstOfMonth.AddDate(0, -1, 0), firstOfMonth, nil
	}
	if len(args) == 0 {
		return monday, monday.AddDate(0, 0, 7), nil
	}
	return rangeFromArgs(args)
}

func newShowCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one entry in detail",
		Long:  "Shows one entry in detail.\n\n" + formatHelp(output.EntryFields()),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			id, err := resolveEntryID(cmd, args[0])
			if err != nil {
				return err
			}
			entry, err := client.Timesheet(cmd.Context(), id)
			if err != nil {
				return err
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}
			return out.renderEntry(output.NewEntry(*entry, l))
		},
	}
	out.register(cmd)
	return cmd
}

// resolveEntryID accepts a numeric ID or the word "current" for the running entry.
func resolveEntryID(cmd *cobra.Command, arg string) (int, error) {
	if arg != "current" {
		return parseID(arg)
	}
	running, err := client.Active(cmd.Context())
	if err != nil {
		return 0, err
	}
	if len(running) == 0 {
		return 0, fmt.Errorf("no running entry")
	}
	return running[0].ID, nil
}
