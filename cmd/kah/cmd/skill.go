package cmd

import (
	"fmt"

	"github.com/kube-agent-helper/kube-agent-helper/cmd/kah/client"
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage skills",
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		var skills []client.Skill
		if err := c.Get(cmd.Context(), "/api/skills", &skills); err != nil {
			return err
		}

		headers := []string{"NAME", "SOURCE", "ENABLED", "PRIORITY"}
		rows := make([][]string, len(skills))
		for i, s := range skills {
			rows[i] = []string{s.Name, s.Source, fmt.Sprintf("%v", s.Enabled), fmt.Sprintf("%d", s.Priority)}
		}
		return printOutput(outputFmt, headers, rows, skills)
	},
}

func init() {
	skillCmd.AddCommand(skillListCmd)
	rootCmd.AddCommand(skillCmd)
}
