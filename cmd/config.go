package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/anned20/kimai-cli/internal/config"
	"github.com/anned20/kimai-cli/internal/kimai"
	"github.com/anned20/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage kimai-cli configuration",
	}
	cmd.AddCommand(newConfigInitCmd(), newConfigShowCmd(), newConfigPathCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the config file interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := &config.Config{TokenCommand: "gopass show -o kimai-token"}
			if existing, err := config.Load(); err == nil {
				c = existing
				// Never write a resolved secret back to disk.
				if c.TokenCommand != "" {
					c.Token = ""
				}
			}

			questions := []*survey.Question{
				{
					Name:     "URL",
					Prompt:   &survey.Input{Message: "Kimai URL", Default: c.URL},
					Validate: survey.Required,
				},
				{
					Name: "TokenCommand",
					Prompt: &survey.Input{
						Message: "Command that prints the API token",
						Default: c.TokenCommand,
					},
				},
			}
			if err := survey.Ask(questions, c); err != nil {
				return err
			}
			c.URL = strings.TrimRight(c.URL, "/")

			if err := c.Save(); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s\n", config.Path())

			// Confirm the credentials work before leaving the user to find out later.
			loaded, err := config.Load()
			if err != nil {
				return err
			}
			probe := kimai.New(loaded.URL, loaded.Token)
			user, err := probe.Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("config saved, but the API rejected it: %w", err)
			}
			serverVersion, err := probe.Version(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "authenticated as %s against Kimai %s\n", user.Username, serverVersion)
			return nil
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := config.Load()
			if err != nil {
				return err
			}
			shown := struct {
				URL             string `json:"url"`
				TokenCommand    string `json:"token_command"`
				TokenResolved   bool   `json:"token_resolved"`
				DefaultProject  int    `json:"default_project"`
				DefaultActivity int    `json:"default_activity"`
				StatusFormat    string `json:"status_format"`
				Interactive     bool   `json:"interactive"`
			}{
				URL:             c.URL,
				TokenCommand:    c.TokenCommand,
				TokenResolved:   c.Token != "",
				DefaultProject:  c.DefaultProject,
				DefaultActivity: c.DefaultActivity,
				StatusFormat:    c.StatusFormat,
				Interactive:     c.Interactive,
			}
			if out.json {
				return output.JSON(os.Stdout, shown)
			}
			fmt.Printf("url               %s\n", shown.URL)
			fmt.Printf("token_command     %s\n", shown.TokenCommand)
			fmt.Printf("token resolved    %t\n", shown.TokenResolved)
			fmt.Printf("default_project   %d\n", shown.DefaultProject)
			fmt.Printf("default_activity  %d\n", shown.DefaultActivity)
			fmt.Printf("status_format     %s\n", shown.StatusFormat)
			fmt.Printf("interactive       %t\n", shown.Interactive)
			return nil
		},
	}
	out.register(cmd)
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(config.Path())
		},
	}
}
