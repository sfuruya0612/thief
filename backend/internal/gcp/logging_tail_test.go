package gcp

import (
	"context"
	"errors"
	"io"
	"testing"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"google.golang.org/protobuf/types/known/anypb"
)

// fakeTailStream は tailStream の手書きモック。responses を順番に Recv で返し、尽きたら
// recvErr を返す。
type fakeTailStream struct {
	sendErr   error
	responses []*loggingpb.TailLogEntriesResponse
	recvErr   error
	recvCalls int
	sendCalls int
}

func (f *fakeTailStream) Send(*loggingpb.TailLogEntriesRequest) error {
	f.sendCalls++
	return f.sendErr
}

func (f *fakeTailStream) Recv() (*loggingpb.TailLogEntriesResponse, error) {
	if f.recvCalls < len(f.responses) {
		resp := f.responses[f.recvCalls]
		f.recvCalls++
		return resp, nil
	}
	return nil, f.recvErr
}

func textLogEntry(text string) *loggingpb.LogEntry {
	return &loggingpb.LogEntry{
		LogName: "projects/p/logs/l",
		Payload: &loggingpb.LogEntry_TextPayload{TextPayload: text},
	}
}

var errSendCallback = errors.New("send callback boom")

func TestRunTailLogEntries(t *testing.T) {
	tests := []struct {
		name          string
		stream        *fakeTailStream
		preCancel     bool
		sendErrAt     int // send コールバックがエラーを返す回数目 (0 なら常に成功)
		wantErrIs     error
		wantSendCalls int
	}{
		{
			name: "send_callback_error_stops_loop",
			stream: &fakeTailStream{
				responses: []*loggingpb.TailLogEntriesResponse{
					{Entries: []*loggingpb.LogEntry{textLogEntry("a"), textLogEntry("b")}},
				},
			},
			sendErrAt:     1,
			wantErrIs:     errSendCallback,
			wantSendCalls: 1,
		},
		{
			name: "recv_error_stops_loop",
			stream: &fakeTailStream{
				recvErr: io.ErrClosedPipe,
			},
			wantErrIs:     io.ErrClosedPipe,
			wantSendCalls: 0,
		},
		{
			name: "context_canceled_stops_loop_before_next_recv",
			stream: &fakeTailStream{
				responses: []*loggingpb.TailLogEntriesResponse{
					{Entries: []*loggingpb.LogEntry{textLogEntry("a")}},
				},
				// cancel 済みなら 2 回目の Recv には到達しないはずなので、到達したことを
				// 検知できるよう io.EOF 以外の値を置いておく。
				recvErr: io.EOF,
			},
			preCancel:     true,
			wantErrIs:     context.Canceled,
			wantSendCalls: 0,
		},
		{
			name: "all_entries_delivered_then_stream_ends",
			stream: &fakeTailStream{
				responses: []*loggingpb.TailLogEntriesResponse{
					{Entries: []*loggingpb.LogEntry{textLogEntry("a")}},
					{Entries: []*loggingpb.LogEntry{textLogEntry("b"), textLogEntry("c")}},
				},
				recvErr: io.EOF,
			},
			wantErrIs:     io.EOF,
			wantSendCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if tt.preCancel {
				cancel()
			}

			sendCalls := 0
			send := func(LogEntryInfo) error {
				sendCalls++
				if tt.sendErrAt != 0 && sendCalls == tt.sendErrAt {
					return errSendCallback
				}
				return nil
			}

			err := runTailLogEntries(ctx, tt.stream, "my-project", "severity=ERROR", send)
			if err == nil {
				t.Fatalf("runTailLogEntries() error = nil, want non-nil")
			}
			if !errors.Is(err, tt.wantErrIs) {
				t.Errorf("runTailLogEntries() error = %v, want to wrap %v", err, tt.wantErrIs)
			}
			if sendCalls != tt.wantSendCalls {
				t.Errorf("send called %d times, want %d", sendCalls, tt.wantSendCalls)
			}
			if tt.stream.sendCalls != 1 {
				t.Errorf("stream.Send called %d times, want exactly 1", tt.stream.sendCalls)
			}
		})
	}
}

func TestLogEntryInfoFromProto(t *testing.T) {
	if _, err := logEntryInfoFromProto(nil); err != nil {
		t.Fatalf("logEntryInfoFromProto(nil) error = %v, want nil", err)
	}

	entry := textLogEntry("hello")
	info, err := logEntryInfoFromProto(entry)
	if err != nil {
		t.Fatalf("logEntryInfoFromProto() error = %v, want nil", err)
	}
	if info.Payload != "hello" {
		t.Errorf("Payload = %q, want %q", info.Payload, "hello")
	}
	if info.LogName != "projects/p/logs/l" {
		t.Errorf("LogName = %q, want %q", info.LogName, "projects/p/logs/l")
	}
}

func TestLogEntryInfoFromProtoInvalidProtoPayload(t *testing.T) {
	entry := &loggingpb.LogEntry{
		Payload: &loggingpb.LogEntry_ProtoPayload{
			ProtoPayload: &anypb.Any{TypeUrl: "type.googleapis.com/does.not.Exist"},
		},
	}
	if _, err := logEntryInfoFromProto(entry); err == nil {
		t.Fatal("logEntryInfoFromProto() error = nil, want error for unregistered proto payload type")
	}
}
