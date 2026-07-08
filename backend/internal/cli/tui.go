package cli

import (
	"github.com/sfuruya0612/thief/backend/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Start the interactive TUI (AWS only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			return tui.Run(cfg)
		},
	}
}
