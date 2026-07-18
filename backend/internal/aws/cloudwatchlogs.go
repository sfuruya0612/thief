package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// LogGroupInfo は CloudWatch Logs のロググループ 1 件を表す。
// API レスポンス (snake_case JSON) と CLI 表示 (ToRow) の両方に使う。
type LogGroupInfo struct {
	Name          string `json:"name"`
	ARN           string `json:"arn"`
	StoredBytes   int64  `json:"stored_bytes"`
	RetentionDays int32  `json:"retention_days"`
	CreationTime  string `json:"creation_time"`
}

// ToRow は CLI のテーブル表示用に 1 行分の文字列スライスを返す。
func (g LogGroupInfo) ToRow() []string {
	retention := "-"
	if g.RetentionDays > 0 {
		retention = fmt.Sprintf("%d", g.RetentionDays)
	}
	return []string{g.Name, fmt.Sprintf("%d", g.StoredBytes), retention, g.CreationTime}
}

// LogEventInfo は CloudWatch Logs のログイベント 1 件を表す。
type LogEventInfo struct {
	Timestamp     string `json:"timestamp"`
	IngestionTime string `json:"ingestion_time"`
	Message       string `json:"message"`
	LogGroup      string `json:"log_group"`
	LogStream     string `json:"log_stream"`
	EventID       string `json:"event_id"`
}

// LogEventPage は 1 ページ分の検索結果。NextPageToken は複数ロググループ横断検索の
// 継続トークン (各グループの nextToken を JSON + base64 でまとめたもの)。
type LogEventPage struct {
	Events        []LogEventInfo `json:"events"`
	NextPageToken string         `json:"next_page_token,omitempty"`
}

// defaultLogEventPerGroupLimit は 1 ロググループ 1 ページあたりの取得上限。
const defaultLogEventPerGroupLimit = 100

// ListLogGroups は指定 profile/region の全ロググループを名前昇順で返す。
func ListLogGroups(ctx context.Context, profile, region string) ([]LogGroupInfo, error) {
	client, err := newCWLogsClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var groups []LogGroupInfo
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(client, &cloudwatchlogs.DescribeLogGroupsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe log groups: %w", err)
		}
		for _, g := range page.LogGroups {
			groups = append(groups, logGroupFromSDK(g))
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}

// FilterLogEvents は選択されたロググループ群を横断してログイベントを検索し、時刻降順で
// 1 ページ分返す。groupIdentifiers はロググループの ARN (末尾 :* を含まない版)。
// pageToken は前回返した NextPageToken (空なら初回)。perGroupLimit が 0 以下なら既定値を使う。
func FilterLogEvents(ctx context.Context, profile, region string, groupIdentifiers []string, pattern, start, end, pageToken string, perGroupLimit int) (*LogEventPage, error) {
	if len(groupIdentifiers) == 0 {
		return &LogEventPage{}, nil
	}
	client, err := newCWLogsClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	if perGroupLimit <= 0 {
		perGroupLimit = defaultLogEventPerGroupLimit
	}

	startMs, err := rfc3339ToMillis(start)
	if err != nil {
		return nil, fmt.Errorf("parse start time: %w", err)
	}
	endMs, err := rfc3339ToMillis(end)
	if err != nil {
		return nil, fmt.Errorf("parse end time: %w", err)
	}

	tokens, err := decodeCompositeToken(pageToken)
	if err != nil {
		return nil, err
	}
	firstPage := pageToken == ""

	// イベントは複数グループから集めるため、ソート用に元の epoch ms を保持する。
	type tsEvent struct {
		ms   int64
		info LogEventInfo
	}
	var collected []tsEvent
	nextTokens := make(map[string]string)

	for _, group := range groupIdentifiers {
		token := tokens[group]
		if !firstPage && token == "" {
			// このグループは前ページまでで取り切っている。
			continue
		}

		in := &cloudwatchlogs.FilterLogEventsInput{
			LogGroupIdentifier: aws.String(group),
			Limit:              aws.Int32(int32(perGroupLimit)),
		}
		if pattern != "" {
			in.FilterPattern = aws.String(pattern)
		}
		if startMs != 0 {
			in.StartTime = aws.Int64(startMs)
			// startFromHead=false (最新優先) は startTime が 2024-01-01 以降のときのみ許可される。
			// UI の期間指定は常に直近のため成立するが、startTime 未指定時は既定 (最古優先) に委ねる。
			in.StartFromHead = aws.Bool(false)
		}
		if endMs != 0 {
			in.EndTime = aws.Int64(endMs)
		}
		if token != "" {
			in.NextToken = aws.String(token)
		}

		out, err := client.FilterLogEvents(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("filter log events (%s): %w", group, err)
		}
		for _, e := range out.Events {
			ms := int64(0)
			if e.Timestamp != nil {
				ms = *e.Timestamp
			}
			collected = append(collected, tsEvent{ms: ms, info: logEventFromSDK(e, group)})
		}
		if out.NextToken != nil && *out.NextToken != "" {
			nextTokens[group] = *out.NextToken
		}
	}

	sort.SliceStable(collected, func(i, j int) bool { return collected[i].ms > collected[j].ms })
	events := make([]LogEventInfo, len(collected))
	for i, c := range collected {
		events[i] = c.info
	}

	nextPageToken, err := encodeCompositeToken(nextTokens)
	if err != nil {
		return nil, err
	}
	return &LogEventPage{Events: events, NextPageToken: nextPageToken}, nil
}

// StartLiveTail は選択されたロググループ群の Live Tail セッションを開き、受信したログイベントを
// send へ 1 件ずつ渡す。send がエラーを返す (ブラウザ切断等) と即座に終了する。ctx のキャンセル、
// ストリーム終了、CloudWatch 側の切断 (概ね数時間) はいずれも non-nil error で戻る。
func StartLiveTail(ctx context.Context, profile, region string, groupIdentifiers []string, pattern string, send func(LogEventInfo) error) error {
	if len(groupIdentifiers) == 0 {
		return fmt.Errorf("start live tail: no log groups selected")
	}
	client, err := newCWLogsClient(ctx, profile, region)
	if err != nil {
		return err
	}

	in := &cloudwatchlogs.StartLiveTailInput{LogGroupIdentifiers: groupIdentifiers}
	if pattern != "" {
		in.LogEventFilterPattern = aws.String(pattern)
	}
	out, err := client.StartLiveTail(ctx, in)
	if err != nil {
		return fmt.Errorf("start live tail: %w", err)
	}
	stream := out.GetStream()
	defer stream.Close()

	for event := range stream.Events() {
		update, ok := event.(*cwltypes.StartLiveTailResponseStreamMemberSessionUpdate)
		if !ok {
			// SessionStart や未知のイベントは無視する。
			continue
		}
		for _, le := range update.Value.SessionResults {
			if err := send(liveTailEventFromSDK(le)); err != nil {
				return err
			}
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("live tail stream: %w", err)
	}
	return nil
}

// logGroupFromSDK は SDK の LogGroup を LogGroupInfo へ変換する。ARN は IAM ポリシー等で
// 参照する末尾 :* を含まない版 (LogGroupArn) を優先し、無ければ Arn から :* を除いて使う。
func logGroupFromSDK(g cwltypes.LogGroup) LogGroupInfo {
	info := LogGroupInfo{}
	if g.LogGroupName != nil {
		info.Name = *g.LogGroupName
	}
	switch {
	case g.LogGroupArn != nil:
		info.ARN = *g.LogGroupArn
	case g.Arn != nil:
		info.ARN = strings.TrimSuffix(*g.Arn, ":*")
	}
	if g.StoredBytes != nil {
		info.StoredBytes = *g.StoredBytes
	}
	if g.RetentionInDays != nil {
		info.RetentionDays = *g.RetentionInDays
	}
	info.CreationTime = millisToRFC3339(g.CreationTime)
	return info
}

// logEventFromSDK は FilterLogEvents のイベントを LogEventInfo へ変換する。
// FilteredLogEvent はロググループ名を持たないため、検索に使った識別子から名前を導出する。
func logEventFromSDK(e cwltypes.FilteredLogEvent, groupIdentifier string) LogEventInfo {
	info := LogEventInfo{
		Timestamp:     millisToRFC3339(e.Timestamp),
		IngestionTime: millisToRFC3339(e.IngestionTime),
		LogGroup:      logGroupNameFromIdentifier(groupIdentifier),
	}
	if e.Message != nil {
		info.Message = *e.Message
	}
	if e.LogStreamName != nil {
		info.LogStream = *e.LogStreamName
	}
	if e.EventId != nil {
		info.EventID = *e.EventId
	}
	return info
}

// liveTailEventFromSDK は Live Tail のイベントを LogEventInfo へ変換する。
func liveTailEventFromSDK(e cwltypes.LiveTailSessionLogEvent) LogEventInfo {
	info := LogEventInfo{
		Timestamp:     millisToRFC3339(e.Timestamp),
		IngestionTime: millisToRFC3339(e.IngestionTime),
	}
	if e.Message != nil {
		info.Message = *e.Message
	}
	if e.LogStreamName != nil {
		info.LogStream = *e.LogStreamName
	}
	if e.LogGroupIdentifier != nil {
		info.LogGroup = logGroupNameFromIdentifier(*e.LogGroupIdentifier)
	}
	return info
}

// logGroupNameFromIdentifier は ARN 形式のロググループ識別子から表示用の名前を取り出す。
// ARN でなければ (既に名前なら) そのまま返す。ARN 形式は末尾 ":log-group:<name>" または
// ":log-group:<name>:*" を含む。
func logGroupNameFromIdentifier(id string) string {
	const marker = ":log-group:"
	idx := strings.Index(id, marker)
	if idx < 0 {
		return id
	}
	name := id[idx+len(marker):]
	return strings.TrimSuffix(name, ":*")
}

// millisToRFC3339 は epoch ミリ秒 (nil 可) を UTC の RFC3339 (ナノ秒精度) 文字列へ変換する。
// nil や 0 は空文字を返す。
func millisToRFC3339(ms *int64) string {
	if ms == nil || *ms == 0 {
		return ""
	}
	return time.UnixMilli(*ms).UTC().Format(time.RFC3339Nano)
}

// rfc3339ToMillis は RFC3339 文字列を epoch ミリ秒へ変換する。空文字は 0 を返す。
func rfc3339ToMillis(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("invalid RFC3339 time %q: %w", s, err)
	}
	return t.UnixMilli(), nil
}

// encodeCompositeToken は各ロググループの nextToken を JSON + base64(URL) でまとめる。
// 空マップは空文字 (これ以上のページなし) を返す。
func encodeCompositeToken(tokens map[string]string) (string, error) {
	if len(tokens) == 0 {
		return "", nil
	}
	b, err := json.Marshal(tokens)
	if err != nil {
		return "", fmt.Errorf("marshal composite page token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// decodeCompositeToken は encodeCompositeToken の逆変換。空文字は nil (初回) を返す。
func decodeCompositeToken(token string) (map[string]string, error) {
	if token == "" {
		return nil, nil
	}
	b, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode composite page token: %w", err)
	}
	var tokens map[string]string
	if err := json.Unmarshal(b, &tokens); err != nil {
		return nil, fmt.Errorf("unmarshal composite page token: %w", err)
	}
	return tokens, nil
}

// newCWLogsClient は CloudWatch Logs API クライアントを生成する。
func newCWLogsClient(ctx context.Context, profile, region string) (*cloudwatchlogs.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *cloudwatchlogs.Client {
		return cloudwatchlogs.NewFromConfig(cfg)
	})
}
