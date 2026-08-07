package aws

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// mockCWLogsFilterEventsClient は cwLogsFilterEventsClient の手書きモック。受け取った Input を
// 呼び出し順に記録し、イベントを含まない空のレスポンスを返す。
type mockCWLogsFilterEventsClient struct {
	inputs []*cloudwatchlogs.FilterLogEventsInput
}

func (m *mockCWLogsFilterEventsClient) FilterLogEvents(_ context.Context, params *cloudwatchlogs.FilterLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	m.inputs = append(m.inputs, params)
	return &cloudwatchlogs.FilterLogEventsOutput{}, nil
}

// TestFilterLogEventsSendsRequestParams は検索条件が FilterLogEventsInput へ設定されることを
// 検証する。期待値は各フィールドの値そのもので書き、未指定の条件では nil のままである
// (= AWS 側の既定に委ねる) ことも固定する。
// StartFromHead は StartTime を指定したときだけ設定される。startFromHead=false (最新優先) は
// startTime が 2024-01-01 以降のときのみ許可される API 制約に合わせた実装であるため、
// 両者が連動することをテストでも固定する。
func TestFilterLogEventsSendsRequestParams(t *testing.T) {
	// 2026-08-07T00:00:00Z / 2026-08-07T01:00:00Z の epoch ミリ秒。
	const (
		startRFC3339 = "2026-08-07T00:00:00Z"
		endRFC3339   = "2026-08-07T01:00:00Z"
	)
	startMs := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC).UnixMilli()
	endMs := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC).UnixMilli()

	tests := []struct {
		name              string
		pattern           string
		start             string
		end               string
		wantFilterPattern *string
		wantStartTime     *int64
		wantEndTime       *int64
		wantStartFromHead *bool
	}{
		{
			name:              "no conditions",
			wantFilterPattern: nil,
			wantStartTime:     nil,
			wantEndTime:       nil,
			wantStartFromHead: nil,
		},
		{
			name:              "pattern only",
			pattern:           "ERROR",
			wantFilterPattern: aws.String("ERROR"),
			wantStartTime:     nil,
			wantEndTime:       nil,
			wantStartFromHead: nil,
		},
		{
			name:              "start only sets start from head",
			start:             startRFC3339,
			wantFilterPattern: nil,
			wantStartTime:     aws.Int64(startMs),
			wantEndTime:       nil,
			wantStartFromHead: aws.Bool(false),
		},
		{
			name:              "end only leaves start from head unset",
			end:               endRFC3339,
			wantFilterPattern: nil,
			wantStartTime:     nil,
			wantEndTime:       aws.Int64(endMs),
			wantStartFromHead: nil,
		},
		{
			name:              "all conditions",
			pattern:           "ERROR",
			start:             startRFC3339,
			end:               endRFC3339,
			wantFilterPattern: aws.String("ERROR"),
			wantStartTime:     aws.Int64(startMs),
			wantEndTime:       aws.Int64(endMs),
			wantStartFromHead: aws.Bool(false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 検索条件はグループごとの全呼び出しへ等しく載る必要があるため 2 グループを渡す。
			// 末尾の perGroupLimit はこのテストの検証対象ではなく、既定値フォールバックへ
			// 落ちない正の値であればよい。
			groups := []string{"arn:aws:logs:ap-northeast-1:123456789012:log-group:app", "arn:aws:logs:ap-northeast-1:123456789012:log-group:worker"}
			client := &mockCWLogsFilterEventsClient{}
			if _, err := filterLogEvents(context.Background(), client, groups, tt.pattern, tt.start, tt.end, "", 50); err != nil {
				t.Fatalf("filterLogEvents: %v", err)
			}
			if len(client.inputs) != len(groups) {
				t.Fatalf("FilterLogEvents called %d times, want %d", len(client.inputs), len(groups))
			}
			for i, in := range client.inputs {
				if diff := cmp.Diff(tt.wantFilterPattern, in.FilterPattern); diff != "" {
					t.Errorf("call %d: FilterPattern mismatch (-want +got):\n%s", i+1, diff)
				}
				if diff := cmp.Diff(tt.wantStartTime, in.StartTime); diff != "" {
					t.Errorf("call %d: StartTime mismatch (-want +got):\n%s", i+1, diff)
				}
				if diff := cmp.Diff(tt.wantEndTime, in.EndTime); diff != "" {
					t.Errorf("call %d: EndTime mismatch (-want +got):\n%s", i+1, diff)
				}
				if diff := cmp.Diff(tt.wantStartFromHead, in.StartFromHead); diff != "" {
					t.Errorf("call %d: StartFromHead mismatch (-want +got):\n%s", i+1, diff)
				}
				// 各呼び出しが意図したグループに対応していること (グループの取り違えや、同じ
				// グループの重複呼び出しが無いこと) の確認。呼び出し回数の検査だけでは、
				// 1 つのグループを 2 回呼んで別のグループを落とす取り違えを検出できない。
				if diff := cmp.Diff(aws.String(groups[i]), in.LogGroupIdentifier); diff != "" {
					t.Errorf("call %d: LogGroupIdentifier mismatch (-want +got):\n%s", i+1, diff)
				}
			}
		})
	}
}

// TestNewStartLiveTailInput は Live Tail の開始リクエストの構築を検証する。
// pattern が空のときにフィルタ無し (LogEventFilterPattern が nil) になることも固定する。
func TestNewStartLiveTailInput(t *testing.T) {
	groups := []string{"arn:aws:logs:ap-northeast-1:123456789012:log-group:app"}

	tests := []struct {
		name    string
		pattern string
		want    *cloudwatchlogs.StartLiveTailInput
	}{
		{
			name:    "pattern given",
			pattern: "ERROR",
			want: &cloudwatchlogs.StartLiveTailInput{
				LogGroupIdentifiers:   groups,
				LogEventFilterPattern: aws.String("ERROR"),
			},
		},
		{
			name:    "pattern empty",
			pattern: "",
			want:    &cloudwatchlogs.StartLiveTailInput{LogGroupIdentifiers: groups},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newStartLiveTailInput(groups, tt.pattern)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(cloudwatchlogs.StartLiveTailInput{})); diff != "" {
				t.Errorf("StartLiveTailInput mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLogGroupNameFromIdentifier(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "arn with trailing wildcard", id: "arn:aws:logs:ap-northeast-1:123456789012:log-group:/aws/lambda/api-handler:*", want: "/aws/lambda/api-handler"},
		{name: "arn without wildcard", id: "arn:aws:logs:ap-northeast-1:123456789012:log-group:/aws/ecs/app", want: "/aws/ecs/app"},
		{name: "bare name", id: "/aws/lambda/api-handler", want: "/aws/lambda/api-handler"},
		{name: "empty", id: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logGroupNameFromIdentifier(tt.id); got != tt.want {
				t.Errorf("logGroupNameFromIdentifier(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestMillisRFC3339RoundTrip(t *testing.T) {
	// 2026-07-18T03:04:05.678Z の epoch ミリ秒。
	const ms int64 = 1784516645678
	rfc := millisToRFC3339(aws.Int64(ms))
	if rfc == "" {
		t.Fatalf("millisToRFC3339(%d) returned empty", ms)
	}
	got, err := rfc3339ToMillis(rfc)
	if err != nil {
		t.Fatalf("rfc3339ToMillis(%q) error: %v", rfc, err)
	}
	if got != ms {
		t.Errorf("round trip = %d, want %d", got, ms)
	}
}

func TestMillisToRFC3339Empty(t *testing.T) {
	if got := millisToRFC3339(nil); got != "" {
		t.Errorf("millisToRFC3339(nil) = %q, want empty", got)
	}
	if got := millisToRFC3339(aws.Int64(0)); got != "" {
		t.Errorf("millisToRFC3339(0) = %q, want empty", got)
	}
}

func TestRFC3339ToMillisErrors(t *testing.T) {
	if got, err := rfc3339ToMillis(""); err != nil || got != 0 {
		t.Errorf("rfc3339ToMillis(\"\") = (%d, %v), want (0, nil)", got, err)
	}
	if _, err := rfc3339ToMillis("not-a-time"); err == nil {
		t.Error("rfc3339ToMillis(\"not-a-time\") expected error, got nil")
	}
}

func TestCompositeTokenRoundTrip(t *testing.T) {
	tokens := map[string]string{
		"arn:aws:logs:ap-northeast-1:1:log-group:/a": "tokenA",
		"arn:aws:logs:ap-northeast-1:1:log-group:/b": "tokenB",
	}
	encoded, err := encodeCompositeToken(tokens)
	if err != nil {
		t.Fatalf("encodeCompositeToken error: %v", err)
	}
	if encoded == "" {
		t.Fatal("encodeCompositeToken returned empty for non-empty map")
	}
	decoded, err := decodeCompositeToken(encoded)
	if err != nil {
		t.Fatalf("decodeCompositeToken error: %v", err)
	}
	if len(decoded) != len(tokens) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(tokens))
	}
	for k, v := range tokens {
		if decoded[k] != v {
			t.Errorf("decoded[%q] = %q, want %q", k, decoded[k], v)
		}
	}
}

func TestCompositeTokenEmpty(t *testing.T) {
	encoded, err := encodeCompositeToken(nil)
	if err != nil || encoded != "" {
		t.Errorf("encodeCompositeToken(nil) = (%q, %v), want (\"\", nil)", encoded, err)
	}
	encoded, err = encodeCompositeToken(map[string]string{})
	if err != nil || encoded != "" {
		t.Errorf("encodeCompositeToken(empty) = (%q, %v), want (\"\", nil)", encoded, err)
	}
	decoded, err := decodeCompositeToken("")
	if err != nil || decoded != nil {
		t.Errorf("decodeCompositeToken(\"\") = (%v, %v), want (nil, nil)", decoded, err)
	}
}

func TestDecodeCompositeTokenMalformed(t *testing.T) {
	if _, err := decodeCompositeToken("!!!not-base64!!!"); err == nil {
		t.Error("decodeCompositeToken(malformed base64) expected error, got nil")
	}
	// 有効な base64 だが JSON ではない場合もエラーになる。
	if _, err := decodeCompositeToken("bm90LWpzb24="); err == nil {
		t.Error("decodeCompositeToken(valid base64, invalid json) expected error, got nil")
	}
}

func TestLogGroupFromSDK(t *testing.T) {
	g := cwltypes.LogGroup{
		LogGroupName:    aws.String("/aws/lambda/api-handler"),
		LogGroupArn:     aws.String("arn:aws:logs:ap-northeast-1:1:log-group:/aws/lambda/api-handler"),
		StoredBytes:     aws.Int64(4096),
		RetentionInDays: aws.Int32(30),
		CreationTime:    aws.Int64(1784516645678),
	}
	got := logGroupFromSDK(g)
	if got.Name != "/aws/lambda/api-handler" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.ARN != "arn:aws:logs:ap-northeast-1:1:log-group:/aws/lambda/api-handler" {
		t.Errorf("ARN = %q", got.ARN)
	}
	if got.StoredBytes != 4096 {
		t.Errorf("StoredBytes = %d", got.StoredBytes)
	}
	if got.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d", got.RetentionDays)
	}
	if got.CreationTime == "" {
		t.Error("CreationTime is empty")
	}
}

func TestLogGroupFromSDKArnFallback(t *testing.T) {
	// LogGroupArn が無い場合は Arn から末尾 :* を除いて使う。
	g := cwltypes.LogGroup{
		LogGroupName: aws.String("/aws/ecs/app"),
		Arn:          aws.String("arn:aws:logs:ap-northeast-1:1:log-group:/aws/ecs/app:*"),
	}
	got := logGroupFromSDK(g)
	if got.ARN != "arn:aws:logs:ap-northeast-1:1:log-group:/aws/ecs/app" {
		t.Errorf("ARN fallback = %q", got.ARN)
	}
}

func TestLogGroupInfoToRow(t *testing.T) {
	tests := []struct {
		name string
		info LogGroupInfo
		want []string
	}{
		{
			name: "with retention",
			info: LogGroupInfo{Name: "/a", StoredBytes: 100, RetentionDays: 7, CreationTime: "2026-07-18T00:00:00Z"},
			want: []string{"/a", "100", "7", "2026-07-18T00:00:00Z"},
		},
		{
			name: "never expire",
			info: LogGroupInfo{Name: "/b", StoredBytes: 0, RetentionDays: 0, CreationTime: ""},
			want: []string{"/b", "0", "-", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.ToRow()
			if len(got) != len(tt.want) {
				t.Fatalf("ToRow len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ToRow()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLogEventFromSDK(t *testing.T) {
	e := cwltypes.FilteredLogEvent{
		EventId:       aws.String("evt-1"),
		Timestamp:     aws.Int64(1784516645678),
		IngestionTime: aws.Int64(1784516645700),
		Message:       aws.String("ERROR boom"),
		LogStreamName: aws.String("2026/07/18/[$LATEST]abc"),
	}
	got := logEventFromSDK(e, "arn:aws:logs:ap-northeast-1:1:log-group:/aws/lambda/api-handler:*")
	if got.EventID != "evt-1" {
		t.Errorf("EventID = %q", got.EventID)
	}
	if got.Message != "ERROR boom" {
		t.Errorf("Message = %q", got.Message)
	}
	if got.LogStream != "2026/07/18/[$LATEST]abc" {
		t.Errorf("LogStream = %q", got.LogStream)
	}
	if got.LogGroup != "/aws/lambda/api-handler" {
		t.Errorf("LogGroup = %q, want derived name", got.LogGroup)
	}
	if got.Timestamp == "" || got.IngestionTime == "" {
		t.Errorf("timestamps empty: %+v", got)
	}
}

func TestLiveTailEventFromSDK(t *testing.T) {
	e := cwltypes.LiveTailSessionLogEvent{
		Timestamp:          aws.Int64(1784516645678),
		IngestionTime:      aws.Int64(1784516645700),
		Message:            aws.String("hello"),
		LogStreamName:      aws.String("stream-1"),
		LogGroupIdentifier: aws.String("arn:aws:logs:ap-northeast-1:1:log-group:/aws/ecs/app"),
	}
	got := liveTailEventFromSDK(e)
	if got.Message != "hello" {
		t.Errorf("Message = %q", got.Message)
	}
	if got.LogGroup != "/aws/ecs/app" {
		t.Errorf("LogGroup = %q", got.LogGroup)
	}
	if got.LogStream != "stream-1" {
		t.Errorf("LogStream = %q", got.LogStream)
	}
}

// liveTailSessionUpdate は SessionUpdate イベントを組み立てるテストヘルパー。
func liveTailSessionUpdate(messages ...string) cwltypes.StartLiveTailResponseStream {
	results := make([]cwltypes.LiveTailSessionLogEvent, len(messages))
	for i, m := range messages {
		results[i] = cwltypes.LiveTailSessionLogEvent{
			Timestamp:     aws.Int64(1784516645678),
			IngestionTime: aws.Int64(1784516645700),
			Message:       aws.String(m),
			LogStreamName: aws.String("stream-1"),
		}
	}
	return &cwltypes.StartLiveTailResponseStreamMemberSessionUpdate{
		Value: cwltypes.LiveTailSessionUpdate{SessionResults: results},
	}
}

func TestRunLiveTailStream(t *testing.T) {
	errSend := errors.New("client disconnected")
	errStream := errors.New("session timed out")

	tests := []struct {
		name string
		// events はクローズ済みチャネルへ事前投入するイベント列。
		events []cwltypes.StartLiveTailResponseStream
		// streamErr はチャネルクローズ後に返すストリームエラー。
		streamErr error
		// sendErr が non-nil なら send は呼ばれるたびにこのエラーを返す。
		sendErr error
		// wantSent は send へ渡ったメッセージの期待列。
		wantSent []string
		// wantErr は返り値のエラーが errors.Is で一致すべきエラー (nil なら正常終了)。
		wantErr error
		// wantWrapped はエラーが "live tail stream:" でラップされて返ることの検査。
		// false でエラーがある場合は、ラップされずに素通しで返ることを検査する。
		wantWrapped bool
	}{
		{
			name:     "SessionUpdate のログが変換されて send へ渡る",
			events:   []cwltypes.StartLiveTailResponseStream{liveTailSessionUpdate("first", "second")},
			wantSent: []string{"first", "second"},
		},
		{
			name: "SessionUpdate 以外のイベントは読み飛ばす",
			events: []cwltypes.StartLiveTailResponseStream{
				&cwltypes.StartLiveTailResponseStreamMemberSessionStart{Value: cwltypes.LiveTailSessionStart{}},
				liveTailSessionUpdate("after start"),
			},
			wantSent: []string{"after start"},
		},
		{
			name:     "send のエラーでループが中断してそのエラーが返る",
			events:   []cwltypes.StartLiveTailResponseStream{liveTailSessionUpdate("first", "second")},
			sendErr:  errSend,
			wantSent: []string{"first"},
			wantErr:  errSend,
		},
		{
			name:        "クローズ後にストリームエラーがあれば伝播する",
			streamErr:   errStream,
			wantErr:     errStream,
			wantWrapped: true,
		},
		{
			name: "クローズ後にストリームエラーが無ければ nil を返す",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan cwltypes.StartLiveTailResponseStream, len(tt.events))
			for _, e := range tt.events {
				events <- e
			}
			close(events)

			var sent []string
			send := func(info LogEventInfo) error {
				sent = append(sent, info.Message)
				return tt.sendErr
			}
			streamErr := func() error { return tt.streamErr }

			err := runLiveTailStream(events, streamErr, send)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantWrapped && !strings.Contains(err.Error(), "live tail stream:") {
				t.Errorf("err = %v, want wrapped with %q", err, "live tail stream:")
			}
			if err != nil && !tt.wantWrapped && strings.Contains(err.Error(), "live tail stream:") {
				t.Errorf("err = %v, want passed through without %q", err, "live tail stream:")
			}
			if len(sent) != len(tt.wantSent) {
				t.Fatalf("sent = %v, want %v", sent, tt.wantSent)
			}
			for i := range sent {
				if sent[i] != tt.wantSent[i] {
					t.Errorf("sent[%d] = %q, want %q", i, sent[i], tt.wantSent[i])
				}
			}
		})
	}
}
