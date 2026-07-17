package gcp

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// defaultLogEntryPageSize はページサイズ未指定時 (0 以下) に使う既定値。
const defaultLogEntryPageSize = 100

// LogEntryInfo はログエントリを一覧表示用に正規化した表現。
type LogEntryInfo struct {
	Timestamp      string            `json:"timestamp"`
	Severity       string            `json:"severity"`
	LogName        string            `json:"log_name"`
	ResourceType   string            `json:"resource_type"`
	ResourceLabels map[string]string `json:"resource_labels,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Payload        string            `json:"payload"`
	InsertID       string            `json:"insert_id"`
	Trace          string            `json:"trace,omitempty"`
}

// LogEntryPage はページ単位のログエントリ取得結果。
type LogEntryPage struct {
	Entries       []LogEntryInfo `json:"entries"`
	NextPageToken string         `json:"next_page_token,omitempty"`
}

// ListLogEntries は指定プロジェクトのログエントリを、利用者フィルターと期間 (RFC3339) を
// AND 結合した条件で絞り込み、新しい順にページ単位で取得する。
// クライアントは呼び出し内で作成・破棄する (他の GCP サービスと同じ規約)。
func ListLogEntries(ctx context.Context, projectID, filter, start, end, pageToken string, pageSize int) (*LogEntryPage, error) {
	if pageSize <= 0 {
		pageSize = defaultLogEntryPageSize
	}

	// WithQuotaProject を指定しない場合、ADC のデフォルト quota project がクオータ判定に
	// 使われ、選択中の projectID と食い違ってしまうため常に明示する (cloudrun.go と同じ規約)。
	client, err := logadmin.NewClient(ctx, projectID, option.WithQuotaProject(projectID))
	if err != nil {
		return nil, fmt.Errorf("create logging admin client: %w", err)
	}
	defer client.Close()

	it := client.Entries(ctx,
		logadmin.Filter(composeLogFilter(filter, start, end)),
		logadmin.NewestFirst(),
	)

	var entries []*logging.Entry
	pager := iterator.NewPager(it, pageSize, pageToken)
	nextToken, err := pager.NextPage(&entries)
	if err != nil {
		return nil, fmt.Errorf("list gcp log entries: %w", err)
	}

	infos := make([]LogEntryInfo, len(entries))
	for i, e := range entries {
		infos[i] = logEntryInfoFromEntry(e)
	}
	return &LogEntryPage{Entries: infos, NextPageToken: nextToken}, nil
}

// composeLogFilter は利用者フィルターと期間条件 (timestamp >= / <=、RFC3339) を AND 結合する
// 純関数。期間の一方だけが指定された場合はそのフィールドの条件のみを追加する。
// 利用者フィルターに OR 等の優先順位に影響する演算子が含まれる場合の括弧補完は行わない
// (Logging query language の解釈は Cloud Logging 側に委ねる)。
func composeLogFilter(filter, start, end string) string {
	var conds []string
	if f := strings.TrimSpace(filter); f != "" {
		conds = append(conds, f)
	}
	if start != "" {
		conds = append(conds, fmt.Sprintf(`timestamp >= %q`, start))
	}
	if end != "" {
		conds = append(conds, fmt.Sprintf(`timestamp <= %q`, end))
	}
	return strings.Join(conds, " AND ")
}

// logEntryInfoFromEntry は logadmin.Client.Entries が返す *logging.Entry を表示用に正規化する。
func logEntryInfoFromEntry(e *logging.Entry) LogEntryInfo {
	if e == nil {
		return LogEntryInfo{}
	}
	info := LogEntryInfo{
		Timestamp: formatTimestamp(e.Timestamp, !e.Timestamp.IsZero()),
		Severity:  e.Severity.String(),
		LogName:   e.LogName,
		Labels:    e.Labels,
		Payload:   payloadToString(e.Payload),
		InsertID:  e.InsertID,
		Trace:     e.Trace,
	}
	if e.Resource != nil {
		info.ResourceType = e.Resource.GetType()
		info.ResourceLabels = e.Resource.GetLabels()
	}
	return info
}

// payloadToString は TextPayload (string) / JSON payload (*structpb.Struct) /
// ProtoPayload (UnmarshalNew 済みの proto.Message) の 3 形態を表示用の文字列へ正規化する。
// structpb.Struct 自体も proto.Message を実装しているため、JSON payload と ProtoPayload は
// どちらも protojson でコンパクトに直列化する (改行・インデントは行わない。整形は frontend の責務)。
func payloadToString(payload any) string {
	switch v := payload.(type) {
	case nil:
		return ""
	case string:
		return v
	case proto.Message:
		b, err := protojson.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}
