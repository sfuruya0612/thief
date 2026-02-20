package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

// toRows converts a slice of Row-implementing items to [][]string for table formatting.
func toRows[T util.Row](items []T) [][]string {
	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = item.ToRow()
	}
	return rows
}

// ListConfig holds the configuration for a generic list command.
type ListConfig[T util.Row] struct {
	Columns  []util.Column
	EmptyMsg string
	Fetch    func(cfg *config.Config) ([]T, error)
}

// runList handles the common pattern of fetching a typed list, checking for empty
// results, and formatting output as a table. It reads config from the command context.
func runList[T util.Row](cmd *cobra.Command, lc ListConfig[T]) error {
	cfg := config.FromContext(cmd.Context())
	items, err := lc.Fetch(cfg)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		cmd.Println(lc.EmptyMsg)
		return nil
	}

	return printRowsOrGroupBy(cfg, lc.Columns, toRows(items))
}

// printRowsOrGroupBy prints rows as a normal table, or groups by cfg.GroupBy columns if set.
// It is used by commands that build [][]string directly and cannot use runList.
func printRowsOrGroupBy(cfg *config.Config, columns []util.Column, rows [][]string) error {
	if cfg.GroupBy != "" {
		groupCols, grouped, err := util.GroupByColumns(columns, rows, cfg.GroupBy)
		if err != nil {
			return err
		}
		f := util.NewTableFormatter(groupCols, cfg.Output)
		if !cfg.NoHeader {
			f.PrintHeader()
		}
		f.PrintRows(grouped)
		return nil
	}

	f := util.NewTableFormatter(columns, cfg.Output)
	if !cfg.NoHeader {
		f.PrintHeader()
	}
	f.PrintRows(rows)
	return nil
}
