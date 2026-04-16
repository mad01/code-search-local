package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set via -ldflags at build time.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print csl version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
