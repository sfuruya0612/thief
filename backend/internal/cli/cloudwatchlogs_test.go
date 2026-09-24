package cli

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
)

// TestLogsEventsCmdArgs は `thief logs events` の引数の形 (位置引数 1 つとフラグの既定値) を
// 固定する。既定の取得期間が変わると、引数なしの実行で取得する範囲が黙って変わる。
func TestLogsEventsCmdArgs(t *testing.T) {
	eventsCmd, _, err := newLogsCmd().Find([]string{"events"})
	if err != nil {
		t.Fatalf("find logs events: %v", err)
	}
	if eventsCmd.Name() != "events" {
		t.Fatalf("command name = %q, want %q", eventsCmd.Name(), "events")
	}

	// ロググループは位置引数で 1 つだけ受け取る。
	if err := eventsCmd.Args(eventsCmd, nil); err == nil {
		t.Error("no positional argument: want an error")
	}
	if err := eventsCmd.Args(eventsCmd, []string{"/aws/lambda/api-handler"}); err != nil {
		t.Errorf("one positional argument = %v, want nil", err)
	}
	if err := eventsCmd.Args(eventsCmd, []string{"/aws/lambda/api-handler", "extra"}); err == nil {
		t.Error("two positional arguments: want an error")
	}

	tests := []struct {
		flag string
		want string
	}{
		{flag: "filter", want: ""},
		{flag: "since", want: time.Hour.String()},
		{flag: "limit", want: "100"},
	}
	for _, tt := range tests {
		f := eventsCmd.Flags().Lookup(tt.flag)
		if f == nil {
			t.Errorf("--%s is not registered", tt.flag)
			continue
		}
		if f.DefValue != tt.want {
			t.Errorf("--%s default = %q, want %q", tt.flag, f.DefValue, tt.want)
		}
	}
}

// TestCWLogsEventsRequestFromFlags はフラグと位置引数が取得条件へ 1 対 1 で写ること、
// 0 以下の --since と 1 から 10000 の範囲外の --limit を AWS へ問い合わせる前に拒否することを検証する。
func TestCWLogsEventsRequestFromFlags(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		want    cwLogsEventsRequest
		wantErr bool
	}
	tests := []testCase{
		{
			name: "no flags use the defaults",
			want: cwLogsEventsRequest{LogGroup: "/aws/lambda/api-handler", Since: time.Hour, Limit: cwLogsDefaultEventLimit},
		},
		{
			name: "all flags",
			args: []string{"--filter", "{ $.level = \"error\" }", "--since", "15m", "--limit", "25"},
			want: cwLogsEventsRequest{LogGroup: "/aws/lambda/api-handler", Filter: "{ $.level = \"error\" }", Since: 15 * time.Minute, Limit: 25},
		},
		{
			name: "limit at the api maximum",
			args: []string{"--limit", "10000"},
			want: cwLogsEventsRequest{LogGroup: "/aws/lambda/api-handler", Since: time.Hour, Limit: cwLogsMaxEventLimit},
		},
		{name: "zero limit", args: []string{"--limit", "0"}, wantErr: true},
		{name: "negative limit", args: []string{"--limit", "-1"}, wantErr: true},
		{name: "limit above the api maximum", args: []string{"--limit", "10001"}, wantErr: true},
		{name: "zero since", args: []string{"--since", "0s"}, wantErr: true},
		{name: "negative since", args: []string{"--since", "-1h"}, wantErr: true},
	}
	// int32 へ変換すると 100 に折り返す値。黙って別の件数にせず拒否する。pflag の Int は
	// int のビット幅で解析するため、この値は int が 64 bit の環境でだけ解析でき、32 bit では
	// 解析エラーになるので、64 bit の環境に限って加える。
	if strconv.IntSize == 64 {
		tests = append(tests, testCase{name: "limit that wraps around int32", args: []string{"--limit", "4294967396"}, wantErr: true})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, _, err := newLogsCmd().Find([]string{"events"})
			if err != nil {
				t.Fatalf("find events subcommand: %v", err)
			}
			if err := sub.ParseFlags(tt.args); err != nil {
				t.Fatalf("parse flags %v: %v", tt.args, err)
			}
			got, err := cwLogsEventsRequestFromFlags(sub, "/aws/lambda/api-handler")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("request = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("request mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// cwLogsFilterEventsCall は差し替えた FilterLogEvents が受け取った引数の記録。
type cwLogsFilterEventsCall struct {
	Profile, Region string
	Groups          []string
	Pattern         string
	Start, End      string
	PageToken       string
	PerGroupLimit   int
}

// TestCWLogsFetchEventsWiring は取得条件が FilterLogEvents の意図した引数に届き、返ったページの
// イベントがそのまま返ることを検証する。開始時刻は now から since を引いた RFC 3339 で固定する。
func TestCWLogsFetchEventsWiring(t *testing.T) {
	now := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	want := []awsinternal.LogEventInfo{{Timestamp: "2026-08-07T00:30:00.000Z", Message: "ERROR boom", LogStream: "s1"}}
	var got cwLogsFilterEventsCall
	fake := func(_ context.Context, profile, region string, groups []string, pattern, start, end, pageToken string, perGroupLimit int) (*awsinternal.LogEventPage, error) {
		got = cwLogsFilterEventsCall{Profile: profile, Region: region, Groups: groups, Pattern: pattern, Start: start, End: end, PageToken: pageToken, PerGroupLimit: perGroupLimit}
		return &awsinternal.LogEventPage{Events: want, NextPageToken: "ignored"}, nil
	}
	cfg := &config.Config{Profile: "dev", Region: "ap-northeast-1"}
	req := cwLogsEventsRequest{LogGroup: "/aws/lambda/api-handler", Filter: "ERROR", Since: 30 * time.Minute, Limit: 25}

	events, err := cwLogsFetchEvents(context.Background(), cfg, req, now, fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := cmp.Diff(want, events); diff != "" {
		t.Errorf("events mismatch (-want +got):\n%s", diff)
	}
	wantCall := cwLogsFilterEventsCall{
		Profile: "dev", Region: "ap-northeast-1",
		Groups:  []string{"/aws/lambda/api-handler"},
		Pattern: "ERROR",
		Start:   "2026-08-07T00:30:00Z",
		End:     "", PageToken: "",
		PerGroupLimit: 25,
	}
	if diff := cmp.Diff(wantCall, got); diff != "" {
		t.Errorf("FilterLogEvents call mismatch (-want +got):\n%s", diff)
	}
}

// TestCWLogsFetchEventsPropagatesError は FilterLogEvents の失敗がそのまま呼び出し元へ返ることを
// 検証する (権限不足やロググループの不存在は AWS のエラーとしてここを通る)。
func TestCWLogsFetchEventsPropagatesError(t *testing.T) {
	sentinel := errors.New("access denied")
	fake := func(context.Context, string, string, []string, string, string, string, string, int) (*awsinternal.LogEventPage, error) {
		return nil, sentinel
	}
	events, err := cwLogsFetchEvents(context.Background(), &config.Config{}, cwLogsEventsRequest{LogGroup: "g", Since: time.Hour, Limit: 1}, time.Now(), fake)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if events != nil {
		t.Errorf("events = %v, want nil", events)
	}
}
