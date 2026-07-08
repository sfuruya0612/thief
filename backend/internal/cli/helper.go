package cli

import (
	"context"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

// loadConfig builds a Config from the command's persistent flags + env + YAML.
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if f := cmd.Flag("profile"); f != nil && f.Changed {
		cfg.Profile = f.Value.String()
	}
	if f := cmd.Flag("region"); f != nil && f.Changed {
		cfg.Region = f.Value.String()
	}
	if f := cmd.Flag("output"); f != nil && f.Changed {
		cfg.Output = f.Value.String()
	}
	if f := cmd.Flag("no-header"); f != nil && f.Changed {
		cfg.NoHeader = f.Value.String() == "true"
	}
	return cfg, nil
}

// runList is a generic helper that fetches resources, formats, and prints them.
func runList[T util.Row](
	cmd *cobra.Command,
	columns []util.Column,
	fetch func(ctx context.Context, profile, region string) ([]T, error),
) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()
	items, err := fetch(ctx, cfg.Profile, cfg.Region)
	if err != nil {
		return err
	}

	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = item.ToRow()
	}

	formatter := util.NewTableFormatter(columns, cfg.Output)
	if !cfg.NoHeader {
		formatter.PrintHeader()
	}
	formatter.PrintRows(rows)
	return nil
}
