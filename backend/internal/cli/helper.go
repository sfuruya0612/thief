package cli

import (
	"context"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

// loadConfig builds a Config from the command's flags + env + YAML.
// フラグは cmd.Flag が nil を返す可能性がある (コマンドごとに定義が異なる) ため、
// 存在しかつ明示的に変更されたものだけを上書きする。
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	override := func(name string, apply func(string)) {
		if f := cmd.Flag(name); f != nil && f.Changed {
			apply(f.Value.String())
		}
	}
	override("profile", func(v string) { cfg.Profile = v })
	override("region", func(v string) { cfg.Region = v })
	override("output", func(v string) { cfg.Output = v })
	override("no-header", func(v string) { cfg.NoHeader = v == "true" })
	override("group-by", func(v string) { cfg.GroupBy = v })
	// Datadog (datadog コマンドの永続フラグ)
	override("site", func(v string) { cfg.Datadog.Site = v })
	override("api-key", func(v string) { cfg.SetDatadogAPIKey(v) })
	override("app-key", func(v string) { cfg.SetDatadogAppKey(v) })
	override("view", func(v string) { cfg.Datadog.View = v })
	override("start-month", func(v string) { cfg.Datadog.StartMonth = v })
	override("end-month", func(v string) { cfg.Datadog.EndMonth = v })
	// TiDB (tidb コマンドの永続フラグ)
	override("public-key", func(v string) { cfg.TiDB.PublicKey = v })
	override("private-key", func(v string) { cfg.SetTiDBPrivateKey(v) })
	override("billed-month", func(v string) { cfg.TiDB.BilledMonth = v })
	// BigQuery (bq コマンドの永続フラグ)
	override("project", func(v string) { cfg.BigQuery.ProjectID = v })
	return cfg, nil
}

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
	Fetch    func(ctx context.Context, cfg *config.Config) ([]T, error)
}

// runList handles the common pattern of fetching a typed list, checking for empty
// results, and formatting output as a table.
func runList[T util.Row](cmd *cobra.Command, lc ListConfig[T]) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	items, err := lc.Fetch(context.Background(), cfg)
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
// runList を使えないコマンド ([][]string を直接組み立てる場合) からも呼ばれる。
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
