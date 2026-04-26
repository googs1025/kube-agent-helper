package cmd

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/kube-agent-helper/kube-agent-helper/cmd/kah/client"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show controller status and summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)

		// Fetch runs to compute summary
		var runs []client.Run
		if err := c.Get(cmd.Context(), "/api/runs?"+url.Values{"limit": []string{"500"}}.Encode(), &runs); err != nil {
			return fmt.Errorf("cannot reach controller at %s: %w", serverURL, err)
		}

		var fixes []client.Fix
		_ = c.Get(cmd.Context(), "/api/fixes?"+url.Values{"limit": []string{"500"}}.Encode(), &fixes)

		var skills []client.Skill
		_ = c.Get(cmd.Context(), "/api/skills", &skills)

		var clusters []client.Cluster
		_ = c.Get(cmd.Context(), "/api/clusters", &clusters)

		// Compute summary
		activeRuns := 0
		for _, r := range runs {
			if r.Status == "Pending" || r.Status == "Running" {
				activeRuns++
			}
		}
		pendingFixes := 0
		for _, f := range fixes {
			if f.Phase == "PendingApproval" || f.Phase == "DryRunComplete" {
				pendingFixes++
			}
		}

		summary := map[string]string{
			"server":       serverURL,
			"status":       "connected",
			"total_runs":   strconv.Itoa(len(runs)),
			"active_runs":  strconv.Itoa(activeRuns),
			"total_fixes":  strconv.Itoa(len(fixes)),
			"pending_fixes": strconv.Itoa(pendingFixes),
			"skills":       strconv.Itoa(len(skills)),
			"clusters":     strconv.Itoa(len(clusters)),
		}

		if outputFmt == "table" {
			fmt.Printf("Server:        %s\n", serverURL)
			fmt.Printf("Status:        connected\n")
			fmt.Printf("Total Runs:    %d\n", len(runs))
			fmt.Printf("Active Runs:   %d\n", activeRuns)
			fmt.Printf("Total Fixes:   %d\n", len(fixes))
			fmt.Printf("Pending Fixes: %d\n", pendingFixes)
			fmt.Printf("Skills:        %d\n", len(skills))
			fmt.Printf("Clusters:      %d\n", len(clusters))
			return nil
		}
		return printOutput(outputFmt, nil, nil, summary)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
