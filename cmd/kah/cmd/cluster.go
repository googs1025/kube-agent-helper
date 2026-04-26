package cmd

import (
	"github.com/kube-agent-helper/kube-agent-helper/cmd/kah/client"
	"github.com/spf13/cobra"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage clusters",
}

var clusterListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured clusters",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		var clusters []client.Cluster
		if err := c.Get(cmd.Context(), "/api/clusters", &clusters); err != nil {
			return err
		}

		headers := []string{"NAME", "PHASE", "DESCRIPTION"}
		rows := make([][]string, len(clusters))
		for i, cl := range clusters {
			rows[i] = []string{cl.Name, cl.Phase, cl.Description}
		}
		return printOutput(outputFmt, headers, rows, clusters)
	},
}

func init() {
	clusterCmd.AddCommand(clusterListCmd)
	rootCmd.AddCommand(clusterCmd)
}
