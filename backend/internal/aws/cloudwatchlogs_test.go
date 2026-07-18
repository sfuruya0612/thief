package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

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
