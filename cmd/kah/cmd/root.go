package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverURL  string
	outputFmt  string
	versionStr string
	commitStr  string
)

// SetVersionInfo sets the version and commit info from ldflags.
func SetVersionInfo(version, commit string) {
	versionStr = version
	commitStr = commit
}

var rootCmd = &cobra.Command{
	Use:   "kah",
	Short: "KubeDoctor CLI",
	Long:  "CLI tool for interacting with the kube-agent-helper controller API",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kah version %s (commit: %s)\n", versionStr, commitStr)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", "", "Controller API URL (env: KAH_SERVER)")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format: table|json|yaml")

	cobra.OnInitialize(initConfig)

	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	if serverURL == "" {
		serverURL = os.Getenv("KAH_SERVER")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
