package cmd

import (
	"fmt"
	"net/url"

	"github.com/kube-agent-helper/kube-agent-helper/cmd/kah/client"
	"github.com/spf13/cobra"
)

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Manage diagnostic fixes",
}

var fixListCmd = &cobra.Command{
	Use:   "list",
	Short: "List diagnostic fixes",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		params := url.Values{}
		if cl, _ := cmd.Flags().GetString("cluster"); cl != "" {
			params.Set("cluster", cl)
		}

		path := "/api/fixes"
		if q := params.Encode(); q != "" {
			path += "?" + q
		}

		var fixes []client.Fix
		if err := c.Get(cmd.Context(), path, &fixes); err != nil {
			return err
		}

		headers := []string{"ID", "NAME", "PHASE", "TARGET", "FINDING"}
		rows := make([][]string, len(fixes))
		for i, f := range fixes {
			id := f.ID
			if len(id) > 8 {
				id = id[:8]
			}
			target := fmt.Sprintf("%s/%s/%s", f.TargetKind, f.TargetNamespace, f.TargetName)
			rows[i] = []string{id, f.Name, f.Phase, target, f.FindingTitle}
		}
		return printOutput(outputFmt, headers, rows, fixes)
	},
}

var fixGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get fix details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		var fix client.Fix
		if err := c.Get(cmd.Context(), "/api/fixes/"+args[0], &fix); err != nil {
			return err
		}
		return printOutput(outputFmt, nil, nil, fix)
	},
}

var fixApproveCmd = &cobra.Command{
	Use:   "approve <id>",
	Short: "Approve a diagnostic fix",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		approvedBy, _ := cmd.Flags().GetString("approved-by")
		body := map[string]string{"approvedBy": approvedBy}
		if err := c.Patch(cmd.Context(), "/api/fixes/"+args[0]+"/approve", body); err != nil {
			return err
		}
		fmt.Printf("Fix %s approved\n", args[0])
		return nil
	},
}

var fixRejectCmd = &cobra.Command{
	Use:   "reject <id>",
	Short: "Reject a diagnostic fix",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		if err := c.Patch(cmd.Context(), "/api/fixes/"+args[0]+"/reject", nil); err != nil {
			return err
		}
		fmt.Printf("Fix %s rejected\n", args[0])
		return nil
	},
}

func init() {
	fixListCmd.Flags().String("cluster", "", "Filter by cluster")

	fixApproveCmd.Flags().String("approved-by", "kah-cli", "Who approved the fix")

	fixCmd.AddCommand(fixListCmd, fixGetCmd, fixApproveCmd, fixRejectCmd)
	rootCmd.AddCommand(fixCmd)
}
