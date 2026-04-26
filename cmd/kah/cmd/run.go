package cmd

import (
	"fmt"
	"net/url"

	"github.com/kube-agent-helper/kube-agent-helper/cmd/kah/client"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Manage diagnostic runs",
}

var runListCmd = &cobra.Command{
	Use:   "list",
	Short: "List diagnostic runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		params := url.Values{}
		if ns, _ := cmd.Flags().GetString("namespace"); ns != "" {
			params.Set("namespace", ns)
		}
		if cl, _ := cmd.Flags().GetString("cluster"); cl != "" {
			params.Set("cluster", cl)
		}
		if ph, _ := cmd.Flags().GetString("phase"); ph != "" {
			params.Set("phase", ph)
		}

		path := "/api/runs"
		if q := params.Encode(); q != "" {
			path += "?" + q
		}

		var runs []client.Run
		if err := c.Get(cmd.Context(), path, &runs); err != nil {
			return err
		}

		headers := []string{"ID", "NAME", "STATUS", "CLUSTER", "CREATED"}
		rows := make([][]string, len(runs))
		for i, r := range runs {
			id := r.ID
			if len(id) > 8 {
				id = id[:8]
			}
			rows[i] = []string{id, r.Name, r.Status, r.ClusterName, r.CreatedAt}
		}
		return printOutput(outputFmt, headers, rows, runs)
	},
}

var runGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get diagnostic run details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		var run client.Run
		if err := c.Get(cmd.Context(), "/api/runs/"+args[0], &run); err != nil {
			return err
		}
		return printOutput(outputFmt, nil, nil, run)
	},
}

var runCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new diagnostic run",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)

		ns, _ := cmd.Flags().GetString("namespace")
		name, _ := cmd.Flags().GetString("name")
		modelConfig, _ := cmd.Flags().GetString("model-config")
		skills, _ := cmd.Flags().GetStringSlice("skills")
		scope, _ := cmd.Flags().GetString("scope")

		body := map[string]interface{}{
			"namespace":     ns,
			"modelConfigRef": modelConfig,
			"target": map[string]interface{}{
				"scope": scope,
			},
		}
		if name != "" {
			body["name"] = name
		}
		if len(skills) > 0 {
			body["skills"] = skills
		}

		var result map[string]interface{}
		if err := c.Post(cmd.Context(), "/api/runs", body, &result); err != nil {
			return err
		}
		fmt.Println("Diagnostic run created successfully")
		return printOutput(outputFmt, nil, nil, result)
	},
}

var runDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a diagnostic run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(serverURL)
		if err := c.Delete(cmd.Context(), "/api/runs/"+args[0]); err != nil {
			return err
		}
		fmt.Printf("Run %s deleted\n", args[0])
		return nil
	},
}

func init() {
	runListCmd.Flags().String("namespace", "", "Filter by namespace")
	runListCmd.Flags().String("cluster", "", "Filter by cluster")
	runListCmd.Flags().String("phase", "", "Filter by phase")

	runCreateCmd.Flags().String("namespace", "default", "Namespace for the run")
	runCreateCmd.Flags().String("name", "", "Name for the run (auto-generated if empty)")
	runCreateCmd.Flags().String("model-config", "", "ModelConfig reference (required)")
	runCreateCmd.Flags().StringSlice("skills", nil, "Skills to run (comma-separated)")
	runCreateCmd.Flags().String("scope", "namespace", "Target scope")
	_ = runCreateCmd.MarkFlagRequired("model-config")

	runCmd.AddCommand(runListCmd, runGetCmd, runCreateCmd, runDeleteCmd)
	rootCmd.AddCommand(runCmd)
}
