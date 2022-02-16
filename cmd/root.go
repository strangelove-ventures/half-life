package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "halflife",
	Short: "Validator monitoring and alerting",
	Long: `Cosmos-based blockchain validator monitoring and alerting utility
	
Checks for scenarios such as:
- Slashing period uptime
- Recent missed blocks (is the validator signing currently)
- Jailed status
- Tombstoned status

Discord messages are created in the configured webhook channel for:
- Current validator status
- Detected alerts
`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {}
