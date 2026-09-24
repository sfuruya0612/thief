package cli

import (
	"context"
	"strings"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var athenaCatalogColumns = []util.Column{
	{Header: "Name"},
	{Header: "Type"},
}

var athenaDatabaseColumns = []util.Column{
	{Header: "Name"},
	{Header: "Description"},
}

var athenaWorkgroupColumns = []util.Column{
	{Header: "Name"},
	{Header: "State"},
	{Header: "Description"},
}

var athenaTableColumns = []util.Column{
	{Header: "Name"},
	{Header: "Type"},
	{Header: "Columns"},
	{Header: "PartitionKeys"},
}

// newAthenaCmd は Athena の一覧取得とクエリ実行のコマンドを返す。
func newAthenaCmd() *cobra.Command {
	athenaCmd := &cobra.Command{
		Use:   "athena",
		Short: "Athena operations",
	}

	catalogsCmd := &cobra.Command{
		Use:   "catalogs",
		Short: "List Athena data catalogs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return athenaRunCatalogs(cmd)
		},
	}

	databasesCmd := &cobra.Command{
		Use:   "databases",
		Short: "List databases in an Athena catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, _ := cmd.Flags().GetString("catalog")
			return athenaRunDatabases(cmd, catalog)
		},
	}
	databasesCmd.Flags().String("catalog", "", "Catalog name (default AwsDataCatalog)")

	workgroupsCmd := &cobra.Command{
		Use:   "workgroups",
		Short: "List Athena workgroups",
		RunE: func(cmd *cobra.Command, args []string) error {
			return athenaRunWorkgroups(cmd)
		},
	}

	tablesCmd := &cobra.Command{
		Use:   "tables",
		Short: "List tables in an Athena database",
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, _ := cmd.Flags().GetString("catalog")
			database, _ := cmd.Flags().GetString("database")
			return athenaRunTables(cmd, catalog, database)
		},
	}
	tablesCmd.Flags().String("catalog", "", "Catalog name (default AwsDataCatalog)")
	tablesCmd.Flags().String("database", "", "Database name (required)")
	_ = tablesCmd.MarkFlagRequired("database")

	queryCmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "Execute a read-only Athena query and print the results",
		Long: "Executes a read-only query and prints the results after it finishes.\n" +
			"The command waits until the query reaches a terminal state; Ctrl-C stops the wait " +
			"but leaves the query running in Athena.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return athenaRunQuery(cmd, athenaQueryInputFromFlags(cmd, args[0]))
		},
	}
	queryCmd.Flags().String("catalog", "", "Catalog name (default AwsDataCatalog)")
	queryCmd.Flags().String("database", "", "Database name")
	queryCmd.Flags().String("workgroup", "", "Workgroup name (default primary)")
	queryCmd.Flags().String("output-location", "", "S3 output location (overrides the workgroup setting)")

	athenaCmd.AddCommand(catalogsCmd, databasesCmd, workgroupsCmd, tablesCmd, queryCmd)
	return athenaCmd
}

func athenaRunCatalogs(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	catalogs, err := awsinternal.ListAthenaCatalogs(commandContext(cmd), cfg.Profile, cfg.Region)
	if err != nil {
		return err
	}
	if len(catalogs) == 0 {
		cmd.Println("No Athena catalogs found")
		return nil
	}
	rows := make([][]string, len(catalogs))
	for i, c := range catalogs {
		rows[i] = []string{c.Name, c.Type}
	}
	return printRowsOrGroupBy(cfg, athenaCatalogColumns, rows)
}

func athenaRunDatabases(cmd *cobra.Command, catalog string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	databases, err := awsinternal.ListAthenaDatabases(commandContext(cmd), cfg.Profile, cfg.Region, catalog)
	if err != nil {
		return err
	}
	if len(databases) == 0 {
		cmd.Println("No Athena databases found")
		return nil
	}
	rows := make([][]string, len(databases))
	for i, d := range databases {
		rows[i] = []string{d.Name, d.Description}
	}
	return printRowsOrGroupBy(cfg, athenaDatabaseColumns, rows)
}

func athenaRunWorkgroups(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	workgroups, err := awsinternal.ListAthenaWorkgroups(commandContext(cmd), cfg.Profile, cfg.Region)
	if err != nil {
		return err
	}
	if len(workgroups) == 0 {
		cmd.Println("No Athena workgroups found")
		return nil
	}
	rows := make([][]string, len(workgroups))
	for i, w := range workgroups {
		rows[i] = []string{w.Name, w.State, w.Description}
	}
	return printRowsOrGroupBy(cfg, athenaWorkgroupColumns, rows)
}

func athenaRunTables(cmd *cobra.Command, catalog, database string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	tables, err := awsinternal.ListAthenaTables(commandContext(cmd), cfg.Profile, cfg.Region, catalog, database)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		cmd.Println("No Athena tables found")
		return nil
	}
	rows := make([][]string, len(tables))
	for i, t := range tables {
		rows[i] = []string{t.Name, t.Type, athenaColumnSummary(t.Columns), athenaColumnSummary(t.PartitionKeys)}
	}
	return printRowsOrGroupBy(cfg, athenaTableColumns, rows)
}

// athenaQueryOps は query コマンドが順に呼ぶ Athena 操作の集合。テストで AWS への接続を
// 差し替え、開始 → 完了待ち → 結果取得 → 出力の結線を検証できるよう関数値で持つ。
type athenaQueryOps struct {
	start   func(ctx context.Context, profile, region string, in awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error)
	wait    func(ctx context.Context, profile, region, id string) (*awsinternal.AthenaQueryExecution, error)
	results func(ctx context.Context, profile, region, id string, maxResults int) (*awsinternal.AthenaResultPage, error)
}

// defaultAthenaQueryOps は internal/aws の実装を束ねた本番用の操作集合を返す。
func defaultAthenaQueryOps() athenaQueryOps {
	return athenaQueryOps{
		start:   awsinternal.StartAthenaQuery,
		wait:    awsinternal.WaitAthenaQuery,
		results: awsinternal.GetAthenaQueryResultsAll,
	}
}

// athenaQueryInputFromFlags は query サブコマンドのフラグと SQL 引数から開始パラメータを
// 組み立てる。未指定のフラグは空文字のまま渡し、既定値の扱いは既存の StartAthenaQuery に従う。
func athenaQueryInputFromFlags(cmd *cobra.Command, sql string) awsinternal.StartAthenaQueryInput {
	catalog, _ := cmd.Flags().GetString("catalog")
	database, _ := cmd.Flags().GetString("database")
	workgroup, _ := cmd.Flags().GetString("workgroup")
	outputLocation, _ := cmd.Flags().GetString("output-location")
	return awsinternal.StartAthenaQueryInput{
		SQL:            sql,
		Catalog:        catalog,
		Database:       database,
		Workgroup:      workgroup,
		OutputLocation: outputLocation,
	}
}

func athenaRunQuery(cmd *cobra.Command, in awsinternal.StartAthenaQueryInput) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	return athenaExecuteQuery(commandContext(cmd), cmd, cfg, defaultAthenaQueryOps(), in)
}

// athenaExecuteQuery はクエリを開始し、終端状態まで待ってから全結果を出力する。
// 空の結果は表を出さずにメッセージだけを出す (他の一覧コマンドの空メッセージと同じ扱い)。
func athenaExecuteQuery(ctx context.Context, cmd *cobra.Command, cfg *config.Config, ops athenaQueryOps, in awsinternal.StartAthenaQueryInput) error {
	exec, err := ops.start(ctx, cfg.Profile, cfg.Region, in)
	if err != nil {
		return err
	}
	// 実行が終端状態になるまで待つ。FAILED / CANCELLED はここでエラーになる。
	if _, err := ops.wait(ctx, cfg.Profile, cfg.Region, exec.ID); err != nil {
		return err
	}
	page, err := ops.results(ctx, cfg.Profile, cfg.Region, exec.ID, 0)
	if err != nil {
		return err
	}
	if len(page.Rows) == 0 {
		cmd.Println("Query returned no results")
		return nil
	}
	return printRowsOrGroupBy(cfg, athenaResultColumns(page.Columns), page.Rows)
}

// athenaResultColumns は結果セットのメタデータを表示用の列定義へ変換する。結果の列は
// 実行する SQL によって変わるため固定の列定義を持たない。
func athenaResultColumns(cols []awsinternal.AthenaResultColumn) []util.Column {
	columns := make([]util.Column, len(cols))
	for i, c := range cols {
		columns[i] = util.Column{Header: c.Name}
	}
	return columns
}

// athenaColumnSummary は列定義を "name:type" のカンマ区切りへまとめる。型まで含めるのは、
// 一覧の 1 行でスキーマを把握できるようにするため (型だけの一覧コマンドは別に無い)。
func athenaColumnSummary(cols []awsinternal.AthenaColumn) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = c.Name + ":" + c.Type
	}
	return strings.Join(parts, ",")
}
