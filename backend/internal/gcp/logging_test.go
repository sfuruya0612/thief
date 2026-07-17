package gcp

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestComposeLogFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		start  string
		end    string
		want   string
	}{
		{
			name: "all_empty",
			want: "",
		},
		{
			name:   "filter_only",
			filter: `severity=ERROR`,
			want:   `severity=ERROR`,
		},
		{
			name:  "start_only",
			start: "2026-07-18T00:00:00Z",
			want:  `timestamp >= "2026-07-18T00:00:00Z"`,
		},
		{
			name: "end_only",
			end:  "2026-07-18T01:00:00Z",
			want: `timestamp <= "2026-07-18T01:00:00Z"`,
		},
		{
			name:   "filter_and_start_and_end",
			filter: `severity=ERROR`,
			start:  "2026-07-18T00:00:00Z",
			end:    "2026-07-18T01:00:00Z",
			want:   `severity=ERROR AND timestamp >= "2026-07-18T00:00:00Z" AND timestamp <= "2026-07-18T01:00:00Z"`,
		},
		{
			name:   "filter_with_surrounding_whitespace_is_trimmed",
			filter: "  severity=ERROR  ",
			want:   `severity=ERROR`,
		},
		{
			name:   "whitespace_only_filter_is_treated_as_empty",
			filter: "   ",
			start:  "2026-07-18T00:00:00Z",
			want:   `timestamp >= "2026-07-18T00:00:00Z"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composeLogFilter(tt.filter, tt.start, tt.end)
			if got != tt.want {
				t.Errorf("composeLogFilter(%q, %q, %q) = %q, want %q", tt.filter, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// mustProtojson は protojson.Marshal の結果を文字列で返す。payloadToString の期待値算出専用。
func mustProtojson(t *testing.T, m proto.Message) string {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
	return string(b)
}

func TestPayloadToString(t *testing.T) {
	structPayload, err := structpb.NewStruct(map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("build struct payload: %v", err)
	}
	durationPayload := durationpb.New(2 * time.Second)

	tests := []struct {
		name    string
		payload any
		want    string
	}{
		{
			name:    "nil_payload",
			payload: nil,
			want:    "",
		},
		{
			name:    "text_payload",
			payload: "plain text log line",
			want:    "plain text log line",
		},
		{
			// JSON payload (*structpb.Struct) は proto.Message でもあるため protojson で
			// コンパクトに直列化される。
			name:    "json_payload",
			payload: structPayload,
			want:    mustProtojson(t, structPayload),
		},
		{
			// ProtoPayload (UnmarshalNew 済みの proto.Message) も同様に protojson で直列化する。
			name:    "proto_payload",
			payload: durationPayload,
			want:    mustProtojson(t, durationPayload),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := payloadToString(tt.payload)
			if got != tt.want {
				t.Errorf("payloadToString(%v) = %q, want %q", tt.payload, got, tt.want)
			}
		})
	}
}

func TestLogEntryInfoFromEntry(t *testing.T) {
	got := logEntryInfoFromEntry(nil)
	if diff := cmp.Diff(LogEntryInfo{}, got); diff != "" {
		t.Errorf("logEntryInfoFromEntry(nil) mismatch (-want +got):\n%s", diff)
	}
}
