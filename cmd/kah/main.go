package main

import (
	"os"

	"github.com/kube-agent-helper/kube-agent-helper/cmd/kah/cmd"
)

// Set via ldflags
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
