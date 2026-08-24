package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/anned20/kimai-cli/internal/output"
	"github.com/spf13/cobra"
)

func newProjectCmd() *cobra.Command {
	var (
		out    outputFlags
		hidden bool
	)
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"projects"},
		Short:   "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}

			type row struct {
				ID       int    `json:"id"`
				Name     string `json:"name"`
				Customer string `json:"customer"`
				Visible  bool   `json:"visible"`
			}
			rows := make([]row, 0, len(l.Projects))
			for _, p := range l.Projects {
				if !p.Visible && !hidden {
					continue
				}
				rows = append(rows, row{p.ID, p.Name, l.CustomerName(p.ID), p.Visible})
			}

			if out.json {
				return output.JSON(os.Stdout, rows)
			}
			if out.format != "" {
				for _, r := range rows {
					if err := output.Template(os.Stdout, out.format, r); err != nil {
						return err
					}
				}
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tCUSTOMER\tPROJECT")
			for _, r := range rows {
				fmt.Fprintf(tw, "%d\t%s\t%s\n", r.ID, r.Customer, r.Name)
			}
			return tw.Flush()
		},
	}
	out.register(cmd)
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include hidden projects")
	return cmd
}

func newCustomerCmd() *cobra.Command {
	var (
		out    outputFlags
		hidden bool
	)
	cmd := &cobra.Command{
		Use:     "customer",
		Aliases: []string{"customers", "client"},
		Short:   "List customers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			l, err := lookup(cmd.Context())
			if err != nil {
				return err
			}
			customers := l.Customers
			if !hidden {
				visible := customers[:0:0]
				for _, c := range customers {
					if c.Visible {
						visible = append(visible, c)
					}
				}
				customers = visible
			}

			if out.json {
				return output.JSON(os.Stdout, customers)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tCUSTOMER")
			for _, c := range customers {
				fmt.Fprintf(tw, "%d\t%s\n", c.ID, c.Name)
			}
			return tw.Flush()
		},
	}
	out.register(cmd)
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include hidden customers")
	return cmd
}

func newActivityCmd() *cobra.Command {
	var (
		out     outputFlags
		project string
		hidden  bool
	)
	cmd := &cobra.Command{
		Use:     "activity",
		Aliases: []string{"activities", "task"},
		Short:   "List activities",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			projectID := 0
			if project != "" {
				l, err := lookup(cmd.Context())
				if err != nil {
					return err
				}
				p, err := l.FindProject(project)
				if err != nil {
					return err
				}
				projectID = p.ID
			}
			activities, err := client.Activities(cmd.Context(), projectID, !hidden)
			if err != nil {
				return err
			}

			if out.json {
				return output.JSON(os.Stdout, activities)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tACTIVITY\tSCOPE")
			for _, a := range activities {
				scope := "global"
				if a.Project != nil {
					scope = fmt.Sprintf("project %d", *a.Project)
				}
				fmt.Fprintf(tw, "%d\t%s\t%s\n", a.ID, a.Name, scope)
			}
			return tw.Flush()
		},
	}
	out.register(cmd)
	cmd.Flags().StringVarP(&project, "project", "p", "", "limit to one project's activities")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include hidden activities")
	return cmd
}

func newTagCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:     "tag",
		Aliases: []string{"tags"},
		Short:   "List tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			tags, err := client.Tags(cmd.Context())
			if err != nil {
				return err
			}
			if out.json {
				return output.JSON(os.Stdout, tags)
			}
			for _, t := range tags {
				fmt.Println(t)
			}
			return nil
		},
	}
	out.register(cmd)
	return cmd
}

func newMeCmd() *cobra.Command {
	var out outputFlags
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Show the account owning the API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup(); err != nil {
				return err
			}
			user, err := client.Me(cmd.Context())
			if err != nil {
				return err
			}
			if out.json {
				return output.JSON(os.Stdout, user)
			}
			serverVersion, err := client.Version(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("user      %s (#%d)\n", user.Username, user.ID)
			if user.Alias != "" {
				fmt.Printf("alias     %s\n", user.Alias)
			}
			fmt.Printf("email     %s\n", user.Email)
			fmt.Printf("timezone  %s\n", user.Timezone)
			fmt.Printf("instance  %s (Kimai %s)\n", cfg.URL, serverVersion)
			return nil
		},
	}
	out.register(cmd)
	return cmd
}

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion <bash|zsh|fish>",
		Short:     "Generate a shell completion script",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			default:
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			}
		},
	}
	return cmd
}
