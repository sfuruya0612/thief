package gcp

import (
	"context"
	"fmt"

	"cloud.google.com/go/logging"
	loggingv2 "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"google.golang.org/api/option"
)

// tailStream は LoggingServiceV2_TailLogEntriesClient のうち runTailLogEntries が使う
// 部分だけを抜き出したインターフェース。テストではモック実装を注入する。
type tailStream interface {
	Send(*loggingpb.TailLogEntriesRequest) error
	Recv() (*loggingpb.TailLogEntriesResponse, error)
}

// TailLogEntries は Cloud Logging の Live Tail (双方向 gRPC ストリーム) を開き、
// 受信したログエントリを 1 件ずつ send へ渡す。
//
// Cloud Logging 側の制約 (呼び出し側はこれを前提にセッション終了を扱うこと):
//   - レスポンスはサーバ側で数秒単位にバッファリングされてから返る (即時性は保証されない)
//   - tail セッションは概ね 1 時間で強制的に切断される
//   - entries.tail (TailLogEntries) には同時セッション数の quota がある
//
// ストリームが終了 (切断・エラー・ctx cancel) した場合、または send がエラーを返した場合、
// 本関数は non-nil エラーを返してループを終了する。呼び出し側はこれをセッション終了の合図として扱う。
func TailLogEntries(ctx context.Context, projectID, filter string, send func(LogEntryInfo) error) error {
	// WithQuotaProject を指定しない場合、ADC のデフォルト quota project がクオータ判定に
	// 使われ、選択中の projectID と食い違ってしまうため常に明示する (cloudrun.go と同じ規約)。
	client, err := loggingv2.NewClient(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return fmt.Errorf("create logging v2 client: %w", err)
	}
	defer client.Close()

	stream, err := client.TailLogEntries(ctx)
	if err != nil {
		return fmt.Errorf("open tail log entries stream: %w", err)
	}

	return runTailLogEntries(ctx, stream, projectID, filter, send)
}

// runTailLogEntries は tailStream に対して 1 回だけ TailLogEntriesRequest を送信し、
// Recv ループで受信したエントリを send へ渡す。TailLogEntries から実際の gRPC 接続処理を
// 切り出したもので、テストではモックの tailStream を注入して終了経路を検証する。
func runTailLogEntries(ctx context.Context, stream tailStream, projectID, filter string, send func(LogEntryInfo) error) error {
	req := &loggingpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + projectID},
		Filter:        filter,
	}
	if err := stream.Send(req); err != nil {
		return fmt.Errorf("send tail log entries request: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("tail log entries context ended: %w", err)
		}

		resp, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv tail log entries response: %w", err)
		}

		for _, le := range resp.GetEntries() {
			info, err := logEntryInfoFromProto(le)
			if err != nil {
				return fmt.Errorf("normalize tail log entry: %w", err)
			}
			if err := send(info); err != nil {
				return fmt.Errorf("send log entry to callback: %w", err)
			}
		}
	}
}

// logEntryInfoFromProto は apiv2 (TailLogEntries) が返す生の *loggingpb.LogEntry を
// logadmin 経由の LogEntryInfo と同じ形へ正規化する。
func logEntryInfoFromProto(le *loggingpb.LogEntry) (LogEntryInfo, error) {
	if le == nil {
		return LogEntryInfo{}, nil
	}

	payload, err := payloadFromProtoLogEntry(le)
	if err != nil {
		return LogEntryInfo{}, err
	}

	info := LogEntryInfo{
		Timestamp: formatTimestamp(le.GetTimestamp().AsTime(), le.GetTimestamp() != nil),
		Severity:  logging.Severity(le.GetSeverity()).String(),
		LogName:   le.GetLogName(),
		Labels:    le.GetLabels(),
		Payload:   payloadToString(payload),
		InsertID:  le.GetInsertId(),
		Trace:     le.GetTrace(),
	}
	if res := le.GetResource(); res != nil {
		info.ResourceType = res.GetType()
		info.ResourceLabels = res.GetLabels()
	}
	return info, nil
}

// payloadFromProtoLogEntry は LogEntry.Payload の oneof (TextPayload/JsonPayload/ProtoPayload)
// を payloadToString がそのまま扱える型へ取り出す。
func payloadFromProtoLogEntry(le *loggingpb.LogEntry) (any, error) {
	switch p := le.GetPayload().(type) {
	case *loggingpb.LogEntry_TextPayload:
		return p.TextPayload, nil
	case *loggingpb.LogEntry_JsonPayload:
		return p.JsonPayload, nil
	case *loggingpb.LogEntry_ProtoPayload:
		msg, err := p.ProtoPayload.UnmarshalNew()
		if err != nil {
			return nil, fmt.Errorf("unmarshal proto payload: %w", err)
		}
		return msg, nil
	default:
		return nil, nil
	}
}
