package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
)

// defaultAthenaCatalog はカタログ未指定時の既定値。
const defaultAthenaCatalog = "AwsDataCatalog"

// athenaResultsMaxDefault は GetQueryResults の 1 ページあたり既定行数 (API 上限は 1000)。
const athenaResultsMaxDefault = 500

// athenaResultsMaxLimit は GetQueryResults の API 上限。
const athenaResultsMaxLimit = 1000

// athenaHistoryMaxDefault は実行履歴の既定取得件数。
const athenaHistoryMaxDefault = 50

// athenaBatchGetMax は BatchGetQueryExecution の 1 回あたり上限件数。
const athenaBatchGetMax = 50

// AthenaCatalog represents an Athena data catalog.
type AthenaCatalog struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// AthenaDatabase represents a database within an Athena data catalog.
type AthenaDatabase struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// AthenaWorkgroup represents an Athena workgroup.
type AthenaWorkgroup struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
}

// AthenaColumn represents a column of an Athena table.
type AthenaColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// AthenaTable represents Glue table metadata including partition keys.
type AthenaTable struct {
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	Columns       []AthenaColumn `json:"columns"`
	PartitionKeys []AthenaColumn `json:"partition_keys"`
}

// AthenaQueryExecution represents the state of an Athena query execution.
type AthenaQueryExecution struct {
	ID             string `json:"id"`
	SQL            string `json:"sql"`
	State          string `json:"state"`
	StateReason    string `json:"state_reason,omitempty"`
	SubmittedAt    string `json:"submitted_at,omitempty"`
	CompletedAt    string `json:"completed_at,omitempty"`
	ElapsedMs      int64  `json:"elapsed_ms"`
	BytesScanned   int64  `json:"bytes_scanned"`
	OutputLocation string `json:"output_location,omitempty"`
	Workgroup      string `json:"workgroup,omitempty"`
	Catalog        string `json:"catalog,omitempty"`
	Database       string `json:"database,omitempty"`
}

// AthenaResultColumn represents a result set column with its Athena type.
type AthenaResultColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// AthenaResultPage holds one page of query results.
type AthenaResultPage struct {
	Columns   []AthenaResultColumn `json:"columns"`
	Rows      [][]string           `json:"rows"`
	NextToken string               `json:"next_token,omitempty"`
}

// StartAthenaQueryInput holds parameters for starting a query execution.
type StartAthenaQueryInput struct {
	SQL            string
	Catalog        string
	Database       string
	Workgroup      string
	OutputLocation string
}

// athenaAPI は Athena SDK クライアントのうち本パッケージが利用する操作の集合。
// テストでは手書きフェイクを差し込む。
type athenaAPI interface {
	ListDataCatalogs(ctx context.Context, params *athena.ListDataCatalogsInput, optFns ...func(*athena.Options)) (*athena.ListDataCatalogsOutput, error)
	ListDatabases(ctx context.Context, params *athena.ListDatabasesInput, optFns ...func(*athena.Options)) (*athena.ListDatabasesOutput, error)
	ListWorkGroups(ctx context.Context, params *athena.ListWorkGroupsInput, optFns ...func(*athena.Options)) (*athena.ListWorkGroupsOutput, error)
	ListTableMetadata(ctx context.Context, params *athena.ListTableMetadataInput, optFns ...func(*athena.Options)) (*athena.ListTableMetadataOutput, error)
	StartQueryExecution(ctx context.Context, params *athena.StartQueryExecutionInput, optFns ...func(*athena.Options)) (*athena.StartQueryExecutionOutput, error)
	GetQueryExecution(ctx context.Context, params *athena.GetQueryExecutionInput, optFns ...func(*athena.Options)) (*athena.GetQueryExecutionOutput, error)
	StopQueryExecution(ctx context.Context, params *athena.StopQueryExecutionInput, optFns ...func(*athena.Options)) (*athena.StopQueryExecutionOutput, error)
	GetQueryResults(ctx context.Context, params *athena.GetQueryResultsInput, optFns ...func(*athena.Options)) (*athena.GetQueryResultsOutput, error)
	ListQueryExecutions(ctx context.Context, params *athena.ListQueryExecutionsInput, optFns ...func(*athena.Options)) (*athena.ListQueryExecutionsOutput, error)
	BatchGetQueryExecution(ctx context.Context, params *athena.BatchGetQueryExecutionInput, optFns ...func(*athena.Options)) (*athena.BatchGetQueryExecutionOutput, error)
}

// ListAthenaCatalogs returns all data catalogs for the given profile/region.
func ListAthenaCatalogs(ctx context.Context, profile, region string) ([]AthenaCatalog, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listAthenaCatalogs(ctx, client)
}

// ListAthenaDatabases returns all databases in the given catalog.
func ListAthenaDatabases(ctx context.Context, profile, region, catalog string) ([]AthenaDatabase, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listAthenaDatabases(ctx, client, catalog)
}

// ListAthenaWorkgroups returns all workgroups for the given profile/region.
func ListAthenaWorkgroups(ctx context.Context, profile, region string) ([]AthenaWorkgroup, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listAthenaWorkgroups(ctx, client)
}

// ListAthenaTables returns table metadata (columns and partition keys) for the
// given catalog/database.
func ListAthenaTables(ctx context.Context, profile, region, catalog, database string) ([]AthenaTable, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listAthenaTables(ctx, client, catalog, database)
}

// StartAthenaQuery starts a read-only query execution and returns its state.
func StartAthenaQuery(ctx context.Context, profile, region string, in StartAthenaQueryInput) (*AthenaQueryExecution, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return startAthenaQuery(ctx, client, in)
}

// GetAthenaQuery returns the current state of the given query execution.
func GetAthenaQuery(ctx context.Context, profile, region, id string) (*AthenaQueryExecution, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return getAthenaQuery(ctx, client, id)
}

// StopAthenaQuery requests cancellation of the given query execution.
func StopAthenaQuery(ctx context.Context, profile, region, id string) error {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return err
	}
	if _, err := client.StopQueryExecution(ctx, &athena.StopQueryExecutionInput{
		QueryExecutionId: aws.String(id),
	}); err != nil {
		return fmt.Errorf("stop athena query execution %s: %w", id, err)
	}
	return nil
}

// GetAthenaQueryResults returns one page of results for a query execution.
func GetAthenaQueryResults(ctx context.Context, profile, region, id, nextToken string, maxResults int) (*AthenaResultPage, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return getAthenaQueryResults(ctx, client, id, nextToken, maxResults)
}

// ListAthenaQueryHistory returns up to maxItems recent query executions
// (newest first) for the given workgroup (empty = primary).
func ListAthenaQueryHistory(ctx context.Context, profile, region, workgroup string, maxItems int) ([]AthenaQueryExecution, error) {
	client, err := newAthenaClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return listAthenaQueryHistory(ctx, client, workgroup, maxItems)
}

func listAthenaCatalogs(ctx context.Context, client athenaAPI) ([]AthenaCatalog, error) {
	var catalogs []AthenaCatalog
	var next *string
	for {
		page, err := client.ListDataCatalogs(ctx, &athena.ListDataCatalogsInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("list athena data catalogs: %w", err)
		}
		for _, c := range page.DataCatalogsSummary {
			catalogs = append(catalogs, AthenaCatalog{
				Name: ptrStr(c.CatalogName),
				Type: string(c.Type),
			})
		}
		if page.NextToken == nil {
			break
		}
		next = page.NextToken
	}
	return catalogs, nil
}

func listAthenaDatabases(ctx context.Context, client athenaAPI, catalog string) ([]AthenaDatabase, error) {
	if catalog == "" {
		catalog = defaultAthenaCatalog
	}
	var databases []AthenaDatabase
	var next *string
	for {
		page, err := client.ListDatabases(ctx, &athena.ListDatabasesInput{
			CatalogName: aws.String(catalog),
			NextToken:   next,
		})
		if err != nil {
			return nil, fmt.Errorf("list athena databases in %s: %w", catalog, err)
		}
		for _, d := range page.DatabaseList {
			databases = append(databases, AthenaDatabase{
				Name:        ptrStr(d.Name),
				Description: ptrStr(d.Description),
			})
		}
		if page.NextToken == nil {
			break
		}
		next = page.NextToken
	}
	return databases, nil
}

func listAthenaWorkgroups(ctx context.Context, client athenaAPI) ([]AthenaWorkgroup, error) {
	var workgroups []AthenaWorkgroup
	var next *string
	for {
		page, err := client.ListWorkGroups(ctx, &athena.ListWorkGroupsInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("list athena workgroups: %w", err)
		}
		for _, w := range page.WorkGroups {
			workgroups = append(workgroups, AthenaWorkgroup{
				Name:        ptrStr(w.Name),
				State:       string(w.State),
				Description: ptrStr(w.Description),
			})
		}
		if page.NextToken == nil {
			break
		}
		next = page.NextToken
	}
	return workgroups, nil
}

func listAthenaTables(ctx context.Context, client athenaAPI, catalog, database string) ([]AthenaTable, error) {
	if catalog == "" {
		catalog = defaultAthenaCatalog
	}
	var tables []AthenaTable
	var next *string
	for {
		page, err := client.ListTableMetadata(ctx, &athena.ListTableMetadataInput{
			CatalogName:  aws.String(catalog),
			DatabaseName: aws.String(database),
			NextToken:    next,
		})
		if err != nil {
			return nil, fmt.Errorf("list athena table metadata in %s.%s: %w", catalog, database, err)
		}
		for _, t := range page.TableMetadataList {
			tables = append(tables, athenaTableFromMetadata(t))
		}
		if page.NextToken == nil {
			break
		}
		next = page.NextToken
	}
	return tables, nil
}

func startAthenaQuery(ctx context.Context, client athenaAPI, in StartAthenaQueryInput) (*AthenaQueryExecution, error) {
	if err := sqlguard.ValidateReadOnly(in.SQL); err != nil {
		return nil, err
	}
	input := &athena.StartQueryExecutionInput{QueryString: aws.String(in.SQL)}
	if in.Catalog != "" || in.Database != "" {
		qec := &athenatypes.QueryExecutionContext{}
		if in.Catalog != "" {
			qec.Catalog = aws.String(in.Catalog)
		}
		if in.Database != "" {
			qec.Database = aws.String(in.Database)
		}
		input.QueryExecutionContext = qec
	}
	if in.Workgroup != "" {
		input.WorkGroup = aws.String(in.Workgroup)
	}
	if in.OutputLocation != "" {
		input.ResultConfiguration = &athenatypes.ResultConfiguration{
			OutputLocation: aws.String(in.OutputLocation),
		}
	}
	out, err := client.StartQueryExecution(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("start athena query execution: %w", err)
	}
	// 開始直後に GetQueryExecution を引き、SQL・出力先を含む正規化済みの実行情報を返す。
	return getAthenaQuery(ctx, client, ptrStr(out.QueryExecutionId))
}

func getAthenaQuery(ctx context.Context, client athenaAPI, id string) (*AthenaQueryExecution, error) {
	out, err := client.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{
		QueryExecutionId: aws.String(id),
	})
	if err != nil {
		return nil, fmt.Errorf("get athena query execution %s: %w", id, err)
	}
	if out.QueryExecution == nil {
		return nil, fmt.Errorf("get athena query execution %s: empty response", id)
	}
	exec := athenaExecutionFrom(*out.QueryExecution)
	return &exec, nil
}

func getAthenaQueryResults(ctx context.Context, client athenaAPI, id, nextToken string, maxResults int) (*AthenaResultPage, error) {
	if maxResults <= 0 || maxResults > athenaResultsMaxLimit {
		maxResults = athenaResultsMaxDefault
	}
	input := &athena.GetQueryResultsInput{
		QueryExecutionId: aws.String(id),
		MaxResults:       aws.Int32(int32(maxResults)),
	}
	if nextToken != "" {
		input.NextToken = aws.String(nextToken)
	}
	out, err := client.GetQueryResults(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get athena query results for %s: %w", id, err)
	}
	return athenaResultPageFrom(out, nextToken == ""), nil
}

func listAthenaQueryHistory(ctx context.Context, client athenaAPI, workgroup string, maxItems int) ([]AthenaQueryExecution, error) {
	if maxItems <= 0 {
		maxItems = athenaHistoryMaxDefault
	}
	input := &athena.ListQueryExecutionsInput{}
	if workgroup != "" {
		input.WorkGroup = aws.String(workgroup)
	}
	var ids []string
	for len(ids) < maxItems {
		out, err := client.ListQueryExecutions(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list athena query executions: %w", err)
		}
		ids = append(ids, out.QueryExecutionIds...)
		if out.NextToken == nil {
			break
		}
		input.NextToken = out.NextToken
	}
	if len(ids) > maxItems {
		ids = ids[:maxItems]
	}
	if len(ids) == 0 {
		return nil, nil
	}

	byID := make(map[string]AthenaQueryExecution, len(ids))
	for start := 0; start < len(ids); start += athenaBatchGetMax {
		end := min(start+athenaBatchGetMax, len(ids))
		out, err := client.BatchGetQueryExecution(ctx, &athena.BatchGetQueryExecutionInput{
			QueryExecutionIds: ids[start:end],
		})
		if err != nil {
			return nil, fmt.Errorf("batch get athena query executions: %w", err)
		}
		for _, qe := range out.QueryExecutions {
			e := athenaExecutionFrom(qe)
			byID[e.ID] = e
		}
	}

	// ListQueryExecutions の返却順 (新しい順) を保って組み立てる。
	items := make([]AthenaQueryExecution, 0, len(ids))
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			items = append(items, e)
		}
	}
	return items, nil
}

// athenaTableFromMetadata converts SDK table metadata into AthenaTable.
func athenaTableFromMetadata(t athenatypes.TableMetadata) AthenaTable {
	table := AthenaTable{Name: ptrStr(t.Name), Type: ptrStr(t.TableType)}
	for _, c := range t.Columns {
		table.Columns = append(table.Columns, AthenaColumn{Name: ptrStr(c.Name), Type: ptrStr(c.Type)})
	}
	for _, c := range t.PartitionKeys {
		table.PartitionKeys = append(table.PartitionKeys, AthenaColumn{Name: ptrStr(c.Name), Type: ptrStr(c.Type)})
	}
	return table
}

// athenaExecutionFrom converts an SDK query execution into the API shape.
func athenaExecutionFrom(qe athenatypes.QueryExecution) AthenaQueryExecution {
	e := AthenaQueryExecution{
		ID:        ptrStr(qe.QueryExecutionId),
		SQL:       ptrStr(qe.Query),
		Workgroup: ptrStr(qe.WorkGroup),
	}
	if qe.QueryExecutionContext != nil {
		e.Catalog = ptrStr(qe.QueryExecutionContext.Catalog)
		e.Database = ptrStr(qe.QueryExecutionContext.Database)
	}
	if qe.ResultConfiguration != nil {
		e.OutputLocation = ptrStr(qe.ResultConfiguration.OutputLocation)
	}
	if qe.Status != nil {
		e.State = string(qe.Status.State)
		e.StateReason = ptrStr(qe.Status.StateChangeReason)
		if qe.Status.SubmissionDateTime != nil {
			e.SubmittedAt = qe.Status.SubmissionDateTime.UTC().Format(time.RFC3339)
		}
		if qe.Status.CompletionDateTime != nil {
			e.CompletedAt = qe.Status.CompletionDateTime.UTC().Format(time.RFC3339)
		}
	}
	if qe.Statistics != nil {
		e.ElapsedMs = ptrInt64(qe.Statistics.TotalExecutionTimeInMillis)
		e.BytesScanned = ptrInt64(qe.Statistics.DataScannedInBytes)
	}
	return e
}

// athenaResultPageFrom converts a results page. firstPage が true のとき、SELECT 結果の
// 先頭に含まれるヘッダ行 (全セルがカラム名と一致する行) を取り除く。
func athenaResultPageFrom(out *athena.GetQueryResultsOutput, firstPage bool) *AthenaResultPage {
	page := &AthenaResultPage{NextToken: ptrStr(out.NextToken)}
	if out.ResultSet == nil {
		return page
	}
	if md := out.ResultSet.ResultSetMetadata; md != nil {
		for _, c := range md.ColumnInfo {
			page.Columns = append(page.Columns, AthenaResultColumn{
				Name: ptrStr(c.Name),
				Type: ptrStr(c.Type),
			})
		}
	}
	rows := out.ResultSet.Rows
	if firstPage && len(rows) > 0 && athenaRowMatchesHeader(rows[0], page.Columns) {
		rows = rows[1:]
	}
	for _, r := range rows {
		row := make([]string, len(r.Data))
		for i, d := range r.Data {
			row[i] = ptrStr(d.VarCharValue)
		}
		page.Rows = append(page.Rows, row)
	}
	return page
}

// athenaRowMatchesHeader は行の全セルがカラム名と一致するかを返す。
func athenaRowMatchesHeader(row athenatypes.Row, cols []AthenaResultColumn) bool {
	if len(cols) == 0 || len(row.Data) != len(cols) {
		return false
	}
	for i, d := range row.Data {
		if ptrStr(d.VarCharValue) != cols[i].Name {
			return false
		}
	}
	return true
}

// newAthenaClient は Athena API クライアントを生成する。
func newAthenaClient(ctx context.Context, profile, region string) (*athena.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *athena.Client {
		return athena.NewFromConfig(cfg)
	})
}
