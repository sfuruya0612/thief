package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/google/go-cmp/cmp"
	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
)

// fakeAthena は athenaAPI の手書きフェイク。使う操作のみ関数フィールドで差し込む。
type fakeAthena struct {
	listDataCatalogs       func(*athena.ListDataCatalogsInput) (*athena.ListDataCatalogsOutput, error)
	listDatabases          func(*athena.ListDatabasesInput) (*athena.ListDatabasesOutput, error)
	listWorkGroups         func(*athena.ListWorkGroupsInput) (*athena.ListWorkGroupsOutput, error)
	listTableMetadata      func(*athena.ListTableMetadataInput) (*athena.ListTableMetadataOutput, error)
	startQueryExecution    func(*athena.StartQueryExecutionInput) (*athena.StartQueryExecutionOutput, error)
	getQueryExecution      func(*athena.GetQueryExecutionInput) (*athena.GetQueryExecutionOutput, error)
	stopQueryExecution     func(*athena.StopQueryExecutionInput) (*athena.StopQueryExecutionOutput, error)
	getQueryResults        func(*athena.GetQueryResultsInput) (*athena.GetQueryResultsOutput, error)
	listQueryExecutions    func(*athena.ListQueryExecutionsInput) (*athena.ListQueryExecutionsOutput, error)
	batchGetQueryExecution func(*athena.BatchGetQueryExecutionInput) (*athena.BatchGetQueryExecutionOutput, error)
}

func (f *fakeAthena) ListDataCatalogs(_ context.Context, p *athena.ListDataCatalogsInput, _ ...func(*athena.Options)) (*athena.ListDataCatalogsOutput, error) {
	return f.listDataCatalogs(p)
}
func (f *fakeAthena) ListDatabases(_ context.Context, p *athena.ListDatabasesInput, _ ...func(*athena.Options)) (*athena.ListDatabasesOutput, error) {
	return f.listDatabases(p)
}
func (f *fakeAthena) ListWorkGroups(_ context.Context, p *athena.ListWorkGroupsInput, _ ...func(*athena.Options)) (*athena.ListWorkGroupsOutput, error) {
	return f.listWorkGroups(p)
}
func (f *fakeAthena) ListTableMetadata(_ context.Context, p *athena.ListTableMetadataInput, _ ...func(*athena.Options)) (*athena.ListTableMetadataOutput, error) {
	return f.listTableMetadata(p)
}
func (f *fakeAthena) StartQueryExecution(_ context.Context, p *athena.StartQueryExecutionInput, _ ...func(*athena.Options)) (*athena.StartQueryExecutionOutput, error) {
	return f.startQueryExecution(p)
}
func (f *fakeAthena) GetQueryExecution(_ context.Context, p *athena.GetQueryExecutionInput, _ ...func(*athena.Options)) (*athena.GetQueryExecutionOutput, error) {
	return f.getQueryExecution(p)
}
func (f *fakeAthena) StopQueryExecution(_ context.Context, p *athena.StopQueryExecutionInput, _ ...func(*athena.Options)) (*athena.StopQueryExecutionOutput, error) {
	return f.stopQueryExecution(p)
}
func (f *fakeAthena) GetQueryResults(_ context.Context, p *athena.GetQueryResultsInput, _ ...func(*athena.Options)) (*athena.GetQueryResultsOutput, error) {
	return f.getQueryResults(p)
}
func (f *fakeAthena) ListQueryExecutions(_ context.Context, p *athena.ListQueryExecutionsInput, _ ...func(*athena.Options)) (*athena.ListQueryExecutionsOutput, error) {
	return f.listQueryExecutions(p)
}
func (f *fakeAthena) BatchGetQueryExecution(_ context.Context, p *athena.BatchGetQueryExecutionInput, _ ...func(*athena.Options)) (*athena.BatchGetQueryExecutionOutput, error) {
	return f.batchGetQueryExecution(p)
}

func TestAthenaExecutionFrom(t *testing.T) {
	submitted := time.Date(2026, 7, 16, 22, 4, 0, 0, time.UTC)
	completed := submitted.Add(6 * time.Second)

	in := athenatypes.QueryExecution{
		QueryExecutionId: aws.String("exec-1"),
		Query:            aws.String("SELECT 1"),
		WorkGroup:        aws.String("primary"),
		QueryExecutionContext: &athenatypes.QueryExecutionContext{
			Catalog:  aws.String("AwsDataCatalog"),
			Database: aws.String("alb_logs_db"),
		},
		ResultConfiguration: &athenatypes.ResultConfiguration{
			OutputLocation: aws.String("s3://bucket/prefix/"),
		},
		Status: &athenatypes.QueryExecutionStatus{
			State:              athenatypes.QueryExecutionStateSucceeded,
			SubmissionDateTime: &submitted,
			CompletionDateTime: &completed,
		},
		Statistics: &athenatypes.QueryExecutionStatistics{
			TotalExecutionTimeInMillis: aws.Int64(6000),
			DataScannedInBytes:         aws.Int64(890_000_000),
		},
	}
	want := AthenaQueryExecution{
		ID:             "exec-1",
		SQL:            "SELECT 1",
		State:          "SUCCEEDED",
		SubmittedAt:    "2026-07-16T22:04:00Z",
		CompletedAt:    "2026-07-16T22:04:06Z",
		ElapsedMs:      6000,
		BytesScanned:   890_000_000,
		OutputLocation: "s3://bucket/prefix/",
		Workgroup:      "primary",
		Catalog:        "AwsDataCatalog",
		Database:       "alb_logs_db",
	}
	if diff := cmp.Diff(want, athenaExecutionFrom(in)); diff != "" {
		t.Errorf("athenaExecutionFrom mismatch (-want +got):\n%s", diff)
	}
}

func TestAthenaResultPageFrom(t *testing.T) {
	header := athenatypes.Row{Data: []athenatypes.Datum{
		{VarCharValue: aws.String("status")},
		{VarCharValue: aws.String("requests")},
	}}
	data := athenatypes.Row{Data: []athenatypes.Datum{
		{VarCharValue: aws.String("502")},
		{VarCharValue: aws.String("1204")},
	}}
	nullCell := athenatypes.Row{Data: []athenatypes.Datum{
		{VarCharValue: aws.String("503")},
		{VarCharValue: nil},
	}}
	meta := &athenatypes.ResultSetMetadata{ColumnInfo: []athenatypes.ColumnInfo{
		{Name: aws.String("status"), Type: aws.String("integer")},
		{Name: aws.String("requests"), Type: aws.String("bigint")},
	}}

	tests := []struct {
		name      string
		out       *athena.GetQueryResultsOutput
		firstPage bool
		want      *AthenaResultPage
	}{
		{
			name: "first page skips header row",
			out: &athena.GetQueryResultsOutput{
				ResultSet: &athenatypes.ResultSet{ResultSetMetadata: meta, Rows: []athenatypes.Row{header, data, nullCell}},
				NextToken: aws.String("token-2"),
			},
			firstPage: true,
			want: &AthenaResultPage{
				Columns: []AthenaResultColumn{
					{Name: "status", Type: "integer"},
					{Name: "requests", Type: "bigint"},
				},
				Rows:      [][]string{{"502", "1204"}, {"503", ""}},
				NextToken: "token-2",
			},
		},
		{
			name: "second page keeps all rows",
			out: &athena.GetQueryResultsOutput{
				ResultSet: &athenatypes.ResultSet{ResultSetMetadata: meta, Rows: []athenatypes.Row{data}},
			},
			firstPage: false,
			want: &AthenaResultPage{
				Columns: []AthenaResultColumn{
					{Name: "status", Type: "integer"},
					{Name: "requests", Type: "bigint"},
				},
				Rows: [][]string{{"502", "1204"}},
			},
		},
		{
			name: "first page without header row keeps data",
			out: &athena.GetQueryResultsOutput{
				ResultSet: &athenatypes.ResultSet{ResultSetMetadata: meta, Rows: []athenatypes.Row{data}},
			},
			firstPage: true,
			want: &AthenaResultPage{
				Columns: []AthenaResultColumn{
					{Name: "status", Type: "integer"},
					{Name: "requests", Type: "bigint"},
				},
				Rows: [][]string{{"502", "1204"}},
			},
		},
		{
			name:      "empty result set",
			out:       &athena.GetQueryResultsOutput{},
			firstPage: true,
			want:      &AthenaResultPage{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := athenaResultPageFrom(tt.out, tt.firstPage)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("athenaResultPageFrom mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAthenaTableFromMetadata(t *testing.T) {
	in := athenatypes.TableMetadata{
		Name:      aws.String("alb_logs"),
		TableType: aws.String("EXTERNAL_TABLE"),
		Columns: []athenatypes.Column{
			{Name: aws.String("status"), Type: aws.String("int")},
			{Name: aws.String("request_url"), Type: aws.String("string")},
		},
		PartitionKeys: []athenatypes.Column{
			{Name: aws.String("date"), Type: aws.String("string")},
		},
	}
	want := AthenaTable{
		Name: "alb_logs",
		Type: "EXTERNAL_TABLE",
		Columns: []AthenaColumn{
			{Name: "status", Type: "int"},
			{Name: "request_url", Type: "string"},
		},
		PartitionKeys: []AthenaColumn{
			{Name: "date", Type: "string"},
		},
	}
	if diff := cmp.Diff(want, athenaTableFromMetadata(in)); diff != "" {
		t.Errorf("athenaTableFromMetadata mismatch (-want +got):\n%s", diff)
	}
}

func TestStartAthenaQueryRejectsWrites(t *testing.T) {
	client := &fakeAthena{
		startQueryExecution: func(*athena.StartQueryExecutionInput) (*athena.StartQueryExecutionOutput, error) {
			t.Fatal("StartQueryExecution should not be called for write statements")
			return nil, nil
		},
	}
	_, err := startAthenaQuery(context.Background(), client, StartAthenaQueryInput{SQL: "DROP TABLE t"})
	if !errors.Is(err, sqlguard.ErrWriteNotAllowed) {
		t.Fatalf("err = %v, want ErrWriteNotAllowed", err)
	}
}

func TestStartAthenaQueryPassesParameters(t *testing.T) {
	var gotStart *athena.StartQueryExecutionInput
	client := &fakeAthena{
		startQueryExecution: func(p *athena.StartQueryExecutionInput) (*athena.StartQueryExecutionOutput, error) {
			gotStart = p
			return &athena.StartQueryExecutionOutput{QueryExecutionId: aws.String("exec-9")}, nil
		},
		getQueryExecution: func(p *athena.GetQueryExecutionInput) (*athena.GetQueryExecutionOutput, error) {
			if ptrStr(p.QueryExecutionId) != "exec-9" {
				t.Fatalf("GetQueryExecution id = %q, want exec-9", ptrStr(p.QueryExecutionId))
			}
			return &athena.GetQueryExecutionOutput{QueryExecution: &athenatypes.QueryExecution{
				QueryExecutionId: p.QueryExecutionId,
				Status:           &athenatypes.QueryExecutionStatus{State: athenatypes.QueryExecutionStateQueued},
			}}, nil
		},
	}
	exec, err := startAthenaQuery(context.Background(), client, StartAthenaQueryInput{
		SQL:            "SELECT 1",
		Catalog:        "AwsDataCatalog",
		Database:       "db1",
		Workgroup:      "primary",
		OutputLocation: "s3://bucket/out/",
	})
	if err != nil {
		t.Fatalf("startAthenaQuery: %v", err)
	}
	if exec.ID != "exec-9" || exec.State != "QUEUED" {
		t.Errorf("exec = %+v, want ID=exec-9 State=QUEUED", exec)
	}
	if ptrStr(gotStart.QueryString) != "SELECT 1" {
		t.Errorf("QueryString = %q", ptrStr(gotStart.QueryString))
	}
	if gotStart.QueryExecutionContext == nil ||
		ptrStr(gotStart.QueryExecutionContext.Catalog) != "AwsDataCatalog" ||
		ptrStr(gotStart.QueryExecutionContext.Database) != "db1" {
		t.Errorf("QueryExecutionContext = %+v", gotStart.QueryExecutionContext)
	}
	if ptrStr(gotStart.WorkGroup) != "primary" {
		t.Errorf("WorkGroup = %q", ptrStr(gotStart.WorkGroup))
	}
	if gotStart.ResultConfiguration == nil || ptrStr(gotStart.ResultConfiguration.OutputLocation) != "s3://bucket/out/" {
		t.Errorf("ResultConfiguration = %+v", gotStart.ResultConfiguration)
	}
}

func TestStartAthenaQueryOmitsResultConfigurationWithoutOutputLocation(t *testing.T) {
	// 出力先未指定の場合は ResultConfiguration を送らず workgroup 側の設定に委ねる
	var gotStart *athena.StartQueryExecutionInput
	client := &fakeAthena{
		startQueryExecution: func(p *athena.StartQueryExecutionInput) (*athena.StartQueryExecutionOutput, error) {
			gotStart = p
			return &athena.StartQueryExecutionOutput{QueryExecutionId: aws.String("exec-10")}, nil
		},
		getQueryExecution: func(p *athena.GetQueryExecutionInput) (*athena.GetQueryExecutionOutput, error) {
			return &athena.GetQueryExecutionOutput{QueryExecution: &athenatypes.QueryExecution{
				QueryExecutionId: p.QueryExecutionId,
				Status:           &athenatypes.QueryExecutionStatus{State: athenatypes.QueryExecutionStateQueued},
			}}, nil
		},
	}
	if _, err := startAthenaQuery(context.Background(), client, StartAthenaQueryInput{SQL: "SELECT 1"}); err != nil {
		t.Fatalf("startAthenaQuery: %v", err)
	}
	if gotStart.ResultConfiguration != nil {
		t.Errorf("ResultConfiguration = %+v, want nil", gotStart.ResultConfiguration)
	}
}

func TestListAthenaQueryHistoryOrdersAndBatches(t *testing.T) {
	// 60 件の ID を 2 ページで返し、BatchGet が 50 件ずつ呼ばれることを検証する。
	ids := make([]string, 60)
	for i := range ids {
		ids[i] = string(rune('a'+i%26)) + "-" + string(rune('0'+i/26)) + "-id"
	}
	var batchSizes []int
	client := &fakeAthena{
		listQueryExecutions: func(p *athena.ListQueryExecutionsInput) (*athena.ListQueryExecutionsOutput, error) {
			if p.NextToken == nil {
				return &athena.ListQueryExecutionsOutput{
					QueryExecutionIds: ids[:40],
					NextToken:         aws.String("next"),
				}, nil
			}
			return &athena.ListQueryExecutionsOutput{QueryExecutionIds: ids[40:]}, nil
		},
		batchGetQueryExecution: func(p *athena.BatchGetQueryExecutionInput) (*athena.BatchGetQueryExecutionOutput, error) {
			batchSizes = append(batchSizes, len(p.QueryExecutionIds))
			var execs []athenatypes.QueryExecution
			for _, id := range p.QueryExecutionIds {
				execs = append(execs, athenatypes.QueryExecution{
					QueryExecutionId: aws.String(id),
					Status:           &athenatypes.QueryExecutionStatus{State: athenatypes.QueryExecutionStateSucceeded},
				})
			}
			return &athena.BatchGetQueryExecutionOutput{QueryExecutions: execs}, nil
		},
	}

	items, err := listAthenaQueryHistory(context.Background(), client, "primary", 60)
	if err != nil {
		t.Fatalf("listAthenaQueryHistory: %v", err)
	}
	if len(items) != 60 {
		t.Fatalf("len(items) = %d, want 60", len(items))
	}
	// ListQueryExecutions の返却順が保たれること。
	for i, item := range items {
		if item.ID != ids[i] {
			t.Fatalf("items[%d].ID = %q, want %q", i, item.ID, ids[i])
		}
	}
	if diff := cmp.Diff([]int{50, 10}, batchSizes); diff != "" {
		t.Errorf("batch sizes mismatch (-want +got):\n%s", diff)
	}
}

func TestListAthenaQueryHistoryEmpty(t *testing.T) {
	client := &fakeAthena{
		listQueryExecutions: func(*athena.ListQueryExecutionsInput) (*athena.ListQueryExecutionsOutput, error) {
			return &athena.ListQueryExecutionsOutput{}, nil
		},
	}
	items, err := listAthenaQueryHistory(context.Background(), client, "", 0)
	if err != nil {
		t.Fatalf("listAthenaQueryHistory: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestListAthenaTablesPaginatesAndDefaultsCatalog(t *testing.T) {
	var catalogs []string
	client := &fakeAthena{
		listTableMetadata: func(p *athena.ListTableMetadataInput) (*athena.ListTableMetadataOutput, error) {
			catalogs = append(catalogs, ptrStr(p.CatalogName))
			if p.NextToken == nil {
				return &athena.ListTableMetadataOutput{
					TableMetadataList: []athenatypes.TableMetadata{{Name: aws.String("t1")}},
					NextToken:         aws.String("next"),
				}, nil
			}
			return &athena.ListTableMetadataOutput{
				TableMetadataList: []athenatypes.TableMetadata{{Name: aws.String("t2")}},
			}, nil
		},
	}
	tables, err := listAthenaTables(context.Background(), client, "", "db1")
	if err != nil {
		t.Fatalf("listAthenaTables: %v", err)
	}
	if len(tables) != 2 || tables[0].Name != "t1" || tables[1].Name != "t2" {
		t.Errorf("tables = %+v, want t1, t2", tables)
	}
	for _, c := range catalogs {
		if c != defaultAthenaCatalog {
			t.Errorf("catalog = %q, want %q", c, defaultAthenaCatalog)
		}
	}
}

func TestGetAthenaQueryResultsClampsMaxResults(t *testing.T) {
	var gotMax int32
	client := &fakeAthena{
		getQueryResults: func(p *athena.GetQueryResultsInput) (*athena.GetQueryResultsOutput, error) {
			gotMax = *p.MaxResults
			return &athena.GetQueryResultsOutput{}, nil
		},
	}
	if _, err := getAthenaQueryResults(context.Background(), client, "exec-1", "", 100_000); err != nil {
		t.Fatalf("getAthenaQueryResults: %v", err)
	}
	if gotMax != athenaResultsMaxDefault {
		t.Errorf("MaxResults = %d, want %d", gotMax, athenaResultsMaxDefault)
	}
}
