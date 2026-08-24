// Package cmd implements the kimai-cli command tree.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anned20/kimai-cli/internal/config"
	"github.com/anned20/kimai-cli/internal/kimai"
	"github.com/anned20/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags.
var version = "dev"

var (
	interactive bool
	cfg         *config.Config
	client      *kimai.Client
	lookupCache *kimai.Lookup
)

// ExecuteContext runs the root command with a cancellable context.
func ExecuteContext(ctx context.Context) {
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kimai-cli",
		Short: "Track time in Kimai from the command line",
		Long: "kimai-cli starts, stops, edits and reports Kimai time entries.\n" +
			"Listing commands support --json for scripting; status also supports\n" +
			"--format with a Go template for status bars.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false,
		"prompt for anything not given on the command line")

	// Apply the configured interactive default before any command runs. A
	// flag given explicitly still wins, including --interactive=false.
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if cmd.Flags().Changed("interactive") {
			return
		}
		if c, err := config.Load(); err == nil && c.Interactive {
			interactive = true
		}
	}

	root.AddCommand(
		newConfigCmd(), newInCmd(), newOutCmd(), newStatusCmd(), newLogCmd(),
		newReportCmd(), newCloneCmd(), newShowCmd(), newEditCmd(), newDeleteCmd(),
		newManualCmd(), newProjectCmd(), newCustomerCmd(), newActivityCmd(),
		newTagCmd(), newMeCmd(), newCompletionCmd(),
	)
	return root
}

// setup loads config and builds the API client. Commands that talk to Kimai
// call this from their RunE.
func setup() error {
	if client != nil {
		return nil
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	cfg = c
	client = kimai.New(c.URL, c.Token)
	return nil
}

// lookup returns the entity name/ID index, fetching it at most once per run.
func lookup(ctx context.Context) (*kimai.Lookup, error) {
	if lookupCache != nil {
		return lookupCache, nil
	}
	l, err := client.NewLookup(ctx)
	if err != nil {
		return nil, err
	}
	lookupCache = l
	return l, nil
}

// formatHelp describes the fields a --format template may use. It is built
// from the rendered types themselves, so it stays correct as they change.
func formatHelp(fields []string) string {
	return "Fields available to --format:\n  " + strings.Join(fields, " ") +
		"\n\nTemplate functions: truncate, join, upper, lower, default.\n" +
		"Wrap output in {{if .Running}}...{{end}} to print nothing when idle."
}

// outputFlags carries the rendering options shared by listing commands.
type outputFlags struct {
	json   bool
	format string
	quiet  bool
}

func (o *outputFlags) register(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&o.json, "json", false, "output JSON")
	cmd.Flags().StringVar(&o.format, "format", "", "render with a Go template")
	cmd.Flags().BoolVarP(&o.quiet, "quiet", "q", false, "print only entry IDs")
}

// renderEntries writes entries honouring --json, --format and --quiet.
func (o *outputFlags) renderEntries(entries []output.Entry, showDate bool) error {
	switch {
	case o.json:
		return output.JSON(os.Stdout, entries)
	case o.quiet:
		for _, e := range entries {
			fmt.Println(e.ID)
		}
		return nil
	case o.format != "":
		for _, e := range entries {
			if err := output.Template(os.Stdout, o.format, e); err != nil {
				return err
			}
		}
		return nil
	default:
		return output.Table(os.Stdout, entries, showDate)
	}
}

// renderEntry writes a single entry honouring the same flags.
func (o *outputFlags) renderEntry(e output.Entry) error {
	switch {
	case o.json:
		return output.JSON(os.Stdout, e)
	case o.quiet:
		fmt.Println(e.ID)
		return nil
	case o.format != "":
		return output.Template(os.Stdout, o.format, e)
	default:
		printEntry(e)
		return nil
	}
}

// printEntry writes the default human-readable view of one entry.
func printEntry(e output.Entry) {
	state := "stopped"
	if e.Running {
		state = "running"
	}
	fmt.Printf("#%d  %s\n", e.ID, state)
	if e.Description != "" {
		fmt.Printf("  description  %s\n", e.Description)
	}
	fmt.Printf("  project      %s\n", e.Project)
	if e.Customer != "" {
		fmt.Printf("  customer     %s\n", e.Customer)
	}
	fmt.Printf("  activity     %s\n", e.Activity)
	fmt.Printf("  begin        %s\n", e.Begin)
	if e.End != "" {
		fmt.Printf("  end          %s\n", e.End)
	}
	fmt.Printf("  duration     %s\n", e.Duration)
	fmt.Printf("  billable     %t\n", e.Billable)
	if len(e.Tags) > 0 {
		fmt.Printf("  tags         %v\n", e.Tags)
	}
}
