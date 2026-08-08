package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

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

// readWithContext は read を別の goroutine で走らせ、read の結果か ctx のキャンセルの
// どちらか早い方を返す。
//
// 標準入力からの読み取りは ctx を一切見ないため、これを挟まないと入力を待つ間 Ctrl-C が
// 効かない。main が signal.NotifyContext でシグナルの既定の動作 (プロセスの即時終了) を
// 止めているため、シグナルは context のキャンセルとしてしか届かず、それを見ない読み取りは
// 入力が来るまで待ち続ける。
//
// read の goroutine は入力か EOF が来るまで残る。標準入力の読み取りを外から中断する
// 移植性のある方法が無いため避けられない。ctx がキャンセルされるのはシグナルを受けた
// ときだけであり、その場合 CLI は直後に終了するため、この goroutine がプロセスの寿命を
// 超えて残ることはない。チャネルはバッファ 1 にしてあるので、受信側が先に戻っても
// goroutine は送信でブロックせず終了できる。
func readWithContext[T any](ctx context.Context, read func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}

	done := make(chan result, 1)
	go func() {
		value, err := read()
		done <- result{value: value, err: err}
	}()

	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case res := <-done:
		return res.value, res.err
	}
}

// promptSelection は対話式の選択の入力を 1 行分読む。
// 読み取れなかった場合 (空行や EOF) は空文字を返し、選択なしとしての扱いを呼び出し側に委ねる。
// ctx がキャンセルされた場合は入力を待たずにエラーを返す。
func promptSelection(ctx context.Context, in io.Reader) (string, error) {
	return readWithContext(ctx, func() (string, error) {
		var input string
		if _, err := fmt.Fscanln(in, &input); err != nil {
			// 空入力はそのまま扱う。
			return "", nil
		}
		return input, nil
	})
}

// readUpdateValue は値更新コマンド (ssm param put / secretsmanager put) の新しい値を解決する。
// --value フラグが明示指定されていればその値を、そうでなければ stdin 全体を読み、末尾の
// 改行を 1 つだけ取り除いて返す (`echo secret | thief ...` が "secret" を送れるようにし、
// かつ機密値をシェル履歴に残さず渡せるようにするため)。
//
// stdin が端末のまま値を渡し忘れた場合は入力待ちになる。ctx を見て抜けられるようにして
// おかないと、そこから Ctrl-C で戻れない。
func readUpdateValue(ctx context.Context, cmd *cobra.Command, stdin io.Reader) (string, error) {
	if f := cmd.Flag("value"); f != nil && f.Changed {
		return f.Value.String(), nil
	}
	b, err := readWithContext(ctx, func() ([]byte, error) { return io.ReadAll(stdin) })
	if err != nil {
		return "", fmt.Errorf("read value from stdin: %w", err)
	}
	return stripOneTrailingNewline(string(b)), nil
}

// stripOneTrailingNewline は末尾の改行を 1 つだけ取り除く ("\r\n" は 2 バイト、"\n" は 1 バイト)。
// 末尾に複数の改行がある場合は最後の 1 つだけを対象とする。
func stripOneTrailingNewline(s string) string {
	switch {
	case strings.HasSuffix(s, "\r\n"):
		return s[:len(s)-2]
	case strings.HasSuffix(s, "\n"):
		return s[:len(s)-1]
	default:
		return s
	}
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

	items, err := lc.Fetch(commandContext(cmd), cfg)
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
