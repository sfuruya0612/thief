package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

// TestNewAthenaCmdTree は `thief athena` のサブコマンド構成と、一覧・実行に必要な
// フラグが生えていることを確かめる。
func TestNewAthenaCmdTree(t *testing.T) {
	cmd := newAthenaCmd()
	if cmd.Name() != "athena" {
		t.Errorf("command name = %q, want %q", cmd.Name(), "athena")
	}

	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	if diff := cmp.Diff([]string{"catalogs", "databases", "query", "tables", "workgroups"}, names); diff != "" {
		t.Errorf("subcommands mismatch (-want +got):\n%s", diff)
	}

	tests := []struct {
		sub       string
		wantFlags []string
		required  []string
		exactArgs int
	}{
		{sub: "catalogs"},
		{sub: "databases", wantFlags: []string{"catalog"}},
		{sub: "workgroups"},
		{sub: "tables", wantFlags: []string{"catalog", "database"}, required: []string{"database"}},
		{
			sub:       "query",
			wantFlags: []string{"catalog", "database", "workgroup", "output-location"},
			exactArgs: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.sub, func(t *testing.T) {
			sub, _, err := cmd.Find([]string{tt.sub})
			if err != nil {
				t.Fatalf("find subcommand %s: %v", tt.sub, err)
			}
			for _, name := range tt.wantFlags {
				if sub.Flag(name) == nil {
					t.Errorf("--%s is not defined", name)
				}
			}
			for _, name := range tt.required {
				f := sub.Flag(name)
				if f == nil {
					t.Fatalf("--%s is not defined", name)
				}
				if f.Annotations[cobra.BashCompOneRequiredFlag] == nil {
					t.Errorf("--%s is not marked required", name)
				}
			}
			if tt.exactArgs > 0 {
				if err := sub.Args(sub, make([]string, tt.exactArgs)); err != nil {
					t.Errorf("Args with %d argument(s) = %v, want no error", tt.exactArgs, err)
				}
				if err := sub.Args(sub, make([]string, tt.exactArgs-1)); err == nil {
					t.Errorf("Args with %d argument(s) returned no error, want a count error", tt.exactArgs-1)
				}
			}
		})
	}
}

// TestAthenaColumnSummary はテーブル一覧に出す列の要約が "name:type" のカンマ区切りに
// なることを検証する。
func TestAthenaColumnSummary(t *testing.T) {
	tests := []struct {
		name string
		cols []awsinternal.AthenaColumn
		want string
	}{
		{name: "no columns", cols: nil, want: ""},
		{
			name: "single column",
			cols: []awsinternal.AthenaColumn{{Name: "status", Type: "int"}},
			want: "status:int",
		},
		{
			name: "multiple columns keep the order",
			cols: []awsinternal.AthenaColumn{
				{Name: "date", Type: "string"},
				{Name: "status", Type: "int"},
			},
			want: "date:string,status:int",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := athenaColumnSummary(tt.cols); got != tt.want {
				t.Errorf("athenaColumnSummary(%+v) = %q, want %q", tt.cols, got, tt.want)
			}
		})
	}
}

// TestAthenaResultColumns はクエリ結果のメタデータが表示用の列ヘッダへ順序どおりに
// 変換されることを検証する。
func TestAthenaResultColumns(t *testing.T) {
	tests := []struct {
		name string
		cols []awsinternal.AthenaResultColumn
		want []util.Column
	}{
		{name: "no columns", cols: nil, want: []util.Column{}},
		{
			name: "keeps the metadata order",
			cols: []awsinternal.AthenaResultColumn{
				{Name: "status", Type: "integer"},
				{Name: "requests", Type: "bigint"},
			},
			want: []util.Column{{Header: "status"}, {Header: "requests"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := athenaResultColumns(tt.cols)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("athenaResultColumns mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNewRootCmdHasAthena は `thief` から athena コマンドに到達できることを確かめる。
func TestNewRootCmdHasAthena(t *testing.T) {
	cmd, _, err := NewRootCmd().Find([]string{"athena", "query"})
	if err != nil {
		t.Fatalf("find 'thief athena query': %v", err)
	}
	if cmd.Name() != "query" {
		t.Errorf("command name = %q, want %q", cmd.Name(), "query")
	}
}

// TestAthenaTableListOutput はテーブル一覧の列定義と行が tab / CSV の両方で出力できる
// ことを検証する。CSV ではセル内のカンマが引用される。
func TestAthenaTableListOutput(t *testing.T) {
	rows := [][]string{
		{"alb_logs", "EXTERNAL_TABLE", "status:int,requests:bigint", "date:string"},
	}

	t.Run("csv", func(t *testing.T) {
		got := captureStdout(t, func() error {
			return printRowsOrGroupBy(&config.Config{Output: "csv"}, athenaTableColumns, rows)
		})
		want := "Name,Type,Columns,PartitionKeys\n" +
			"alb_logs,EXTERNAL_TABLE,\"status:int,requests:bigint\",date:string\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("tab", func(t *testing.T) {
		got := captureStdout(t, func() error {
			return printRowsOrGroupBy(&config.Config{Output: "tab"}, athenaTableColumns, rows)
		})
		// セル内に空白が無いため、空白区切りの語の列で列ヘッダと行の並びを比べられる。
		want := []string{
			"Name", "Type", "Columns", "PartitionKeys",
			"alb_logs", "EXTERNAL_TABLE", "status:int,requests:bigint", "date:string",
		}
		if diff := cmp.Diff(want, strings.Fields(got)); diff != "" {
			t.Errorf("fields mismatch (-want +got):\n%s", diff)
		}
	})
}

// athenaQueryCall は fakeAthenaQueryOps が記録する呼び出しの内容。
type athenaQueryCall struct {
	op      string
	profile string
	region  string
	id      string
	in      awsinternal.StartAthenaQueryInput
}

// fakeAthenaQueryOps は athenaExecuteQuery の結線を検証するための操作集合を返す。
// 各操作は呼び出しを calls に記録し、与えられた戻り値をそのまま返す。
func fakeAthenaQueryOps(calls *[]athenaQueryCall, startErr, waitErr, resultsErr error, page *awsinternal.AthenaResultPage) athenaQueryOps {
	return athenaQueryOps{
		start: func(_ context.Context, profile, region string, in awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
			*calls = append(*calls, athenaQueryCall{op: "start", profile: profile, region: region, in: in})
			if startErr != nil {
				return nil, startErr
			}
			return &awsinternal.AthenaQueryExecution{ID: "exec-1", State: "QUEUED"}, nil
		},
		wait: func(_ context.Context, profile, region, id string) (*awsinternal.AthenaQueryExecution, error) {
			*calls = append(*calls, athenaQueryCall{op: "wait", profile: profile, region: region, id: id})
			if waitErr != nil {
				return nil, waitErr
			}
			return &awsinternal.AthenaQueryExecution{ID: id, State: "SUCCEEDED"}, nil
		},
		results: func(_ context.Context, profile, region, id string, maxResults int) (*awsinternal.AthenaResultPage, error) {
			*calls = append(*calls, athenaQueryCall{op: "results", profile: profile, region: region, id: id})
			if maxResults != 0 {
				panic("maxResults must be 0 so that internal/aws applies its default page size")
			}
			if resultsErr != nil {
				return nil, resultsErr
			}
			return page, nil
		},
	}
}

// TestAthenaQueryInputFromFlags は query サブコマンドの各フラグが StartAthenaQueryInput の
// 対応するフィールドへ入り、未指定のフラグは空文字のままになることを検証する。
func TestAthenaQueryInputFromFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want awsinternal.StartAthenaQueryInput
	}{
		{name: "no flags", want: awsinternal.StartAthenaQueryInput{SQL: "SELECT 1"}},
		{
			name: "all flags",
			args: []string{"--catalog", "glue", "--database", "logs", "--workgroup", "adhoc", "--output-location", "s3://bucket/results/"},
			want: awsinternal.StartAthenaQueryInput{
				SQL: "SELECT 1", Catalog: "glue", Database: "logs", Workgroup: "adhoc", OutputLocation: "s3://bucket/results/",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, _, err := newAthenaCmd().Find([]string{"query"})
			if err != nil {
				t.Fatalf("find query subcommand: %v", err)
			}
			if err := sub.ParseFlags(tt.args); err != nil {
				t.Fatalf("parse flags %v: %v", tt.args, err)
			}
			got := athenaQueryInputFromFlags(sub, "SELECT 1")
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("input mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAthenaExecuteQueryWiring は開始 → 完了待ち → 結果取得の順に、開始で得た実行 ID と
// 設定のプロファイル / リージョンが引き回され、結果が動的な列で tab / CSV 出力されることを
// 検証する。
func TestAthenaExecuteQueryWiring(t *testing.T) {
	in := awsinternal.StartAthenaQueryInput{
		SQL: "SELECT status, count(*) AS requests FROM alb_logs GROUP BY 1", Database: "logs", Workgroup: "primary",
	}
	page := &awsinternal.AthenaResultPage{
		Columns: []awsinternal.AthenaResultColumn{{Name: "status", Type: "integer"}, {Name: "requests", Type: "bigint"}},
		Rows:    [][]string{{"200", "1204"}, {"502", "7"}},
	}
	wantCalls := []athenaQueryCall{
		{op: "start", profile: "dev", region: "ap-northeast-1", in: in},
		{op: "wait", profile: "dev", region: "ap-northeast-1", id: "exec-1"},
		{op: "results", profile: "dev", region: "ap-northeast-1", id: "exec-1"},
	}

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "csv", output: "csv", want: "status,requests\n200,1204\n502,7\n"},
		{name: "tab", output: "tab", want: "status requests 200 1204 502 7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []athenaQueryCall
			cfg := &config.Config{Profile: "dev", Region: "ap-northeast-1", Output: tt.output}
			got := captureStdout(t, func() error {
				return athenaExecuteQuery(context.Background(), &cobra.Command{}, cfg, fakeAthenaQueryOps(&calls, nil, nil, nil, page), in)
			})
			if tt.output == "tab" {
				got = strings.Join(strings.Fields(got), " ")
			}
			if got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
			if diff := cmp.Diff(wantCalls, calls, cmp.AllowUnexported(athenaQueryCall{})); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAthenaExecuteQueryNoRows は 0 行の結果で表を出さず、メッセージだけを出すことを
// 検証する。
func TestAthenaExecuteQueryNoRows(t *testing.T) {
	var calls []athenaQueryCall
	// cobra の Println は SetOut で設定した writer (未設定なら stderr) へ書く。
	cmd := &cobra.Command{}
	var message bytes.Buffer
	cmd.SetOut(&message)
	page := &awsinternal.AthenaResultPage{Columns: []awsinternal.AthenaResultColumn{{Name: "status", Type: "integer"}}}

	stdout := captureStdout(t, func() error {
		return athenaExecuteQuery(context.Background(), cmd, &config.Config{Output: "tab"}, fakeAthenaQueryOps(&calls, nil, nil, nil, page), awsinternal.StartAthenaQueryInput{SQL: "SELECT 1"})
	})
	if stdout != "" {
		t.Errorf("stdout = %q, want no table output", stdout)
	}
	if got := message.String(); got != "Query returned no results\n" {
		t.Errorf("message = %q, want %q", got, "Query returned no results\n")
	}
}

// TestAthenaExecuteQueryPropagatesErrors は開始・完了待ち・結果取得のそれぞれの失敗が
// そのまま返り、失敗した段より後の操作を呼ばないことを検証する。
func TestAthenaExecuteQueryPropagatesErrors(t *testing.T) {
	wantErr := errors.New("boom")
	tests := []struct {
		name      string
		startErr  error
		waitErr   error
		resultErr error
		wantOps   []string
	}{
		{name: "start fails", startErr: wantErr, wantOps: []string{"start"}},
		{name: "wait fails", waitErr: wantErr, wantOps: []string{"start", "wait"}},
		{name: "results fail", resultErr: wantErr, wantOps: []string{"start", "wait", "results"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []athenaQueryCall
			ops := fakeAthenaQueryOps(&calls, tt.startErr, tt.waitErr, tt.resultErr, &awsinternal.AthenaResultPage{})
			err := athenaExecuteQuery(context.Background(), &cobra.Command{}, &config.Config{Output: "tab"}, ops, awsinternal.StartAthenaQueryInput{SQL: "SELECT 1"})
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
			}
			var ops2 []string
			for _, c := range calls {
				ops2 = append(ops2, c.op)
			}
			if diff := cmp.Diff(tt.wantOps, ops2); diff != "" {
				t.Errorf("operations mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// captureStdout は f の実行中に os.Stdout へ書かれた内容を返す。printRowsOrGroupBy は
// util.TableFormatter 経由で os.Stdout へ直接書くため、出力の検証には差し替えが要る。
// internal/cli のテストは t.Parallel() を使わないため、差し替えが衝突することはない。
func captureStdout(t *testing.T, f func() error) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
		_ = r.Close()
	}()

	if err := f(); err != nil {
		_ = w.Close()
		t.Fatalf("output function: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read output: %v", err)
	}
	return buf.String()
}
