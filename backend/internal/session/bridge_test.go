package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// newTestBrowserPair はブラウザ側 WebSocket のペア (backend が持つ server 側 conn と、
// ブラウザ役の client 側 conn) を確立して返す。
func newTestBrowserPair(ctx context.Context, t *testing.T) (server, client *websocket.Conn) {
	t.Helper()
	conns := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept fake browser websocket: %v", err)
			return
		}
		conns <- conn
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("dial fake browser websocket: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(websocket.StatusNormalClosure, "test done") })

	select {
	case server = <-conns:
		return server, client
	case <-ctx.Done():
		t.Fatal("timed out waiting for browser connection")
		return nil, nil
	}
}

func TestBridgeRunEndToEnd(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dc, agent := openTestDataChannel(ctx, t)
	browserServer, browserClient := newTestBrowserPair(ctx, t)

	bridge := &Bridge{DataChannel: dc, Browser: browserServer}
	runErr := make(chan error, 1)
	go func() { runErr <- bridge.Run(ctx) }()

	// ブラウザは接続直後 (ハンドシェイク完了前) に resize とキー入力を送る (ws.onopen の挙動を再現)。
	if err := browserClient.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","cols":120,"rows":40}`)); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	if err := browserClient.Write(ctx, websocket.MessageBinary, []byte("ls\n")); err != nil {
		t.Fatalf("write input: %v", err)
	}

	// ハンドシェイクを完了させると、HandshakeResponse (seq 0) → resize (seq 1) → 入力 (seq 2) の順で届く。
	resp := completeHandshake(ctx, t, agent)
	if resp.SequenceNumber != 0 || resp.Flags != FlagSyn {
		t.Errorf("handshake response seq = %d flags = %d, want seq 0 with SYN", resp.SequenceNumber, resp.Flags)
	}

	size := readInputStreamData(ctx, t, agent)
	if size.PayloadType != PayloadTypeSize || size.SequenceNumber != 1 {
		t.Errorf("second message payload type = %d seq = %d, want size (%d) seq 1", size.PayloadType, size.SequenceNumber, PayloadTypeSize)
	}
	input := readInputStreamData(ctx, t, agent)
	if input.PayloadType != PayloadTypeOutput || input.SequenceNumber != 2 {
		t.Errorf("third message payload type = %d seq = %d, want output (%d) seq 2", input.PayloadType, input.SequenceNumber, PayloadTypeOutput)
	}
	if string(input.Payload) != "ls\n" {
		t.Errorf("input payload = %q, want %q", input.Payload, "ls\n")
	}

	// agent の出力がブラウザへ BINARY で届くこと。
	sendOutputStreamData(ctx, t, agent, 2, PayloadTypeOutput, []byte("file.txt\n"))
	typ, raw, err := browserClient.Read(ctx)
	if err != nil {
		t.Fatalf("read output on browser: %v", err)
	}
	if typ != websocket.MessageBinary || string(raw) != "file.txt\n" {
		t.Errorf("browser received type = %v payload = %q, want binary %q", typ, raw, "file.txt\n")
	}

	// ブラウザがタブクローズ相当 (StatusGoingAway) で切断したら Run は nil で終了すること。
	if err := browserClient.Close(websocket.StatusGoingAway, ""); err != nil {
		t.Fatalf("close browser: %v", err)
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil on browser going away", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not finish after browser close")
	}
}

func TestBridgeRunReturnsNilOnBrowserNormalClose(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dc, _ := openTestDataChannel(ctx, t)
	browserServer, browserClient := newTestBrowserPair(ctx, t)

	bridge := &Bridge{DataChannel: dc, Browser: browserServer}
	runErr := make(chan error, 1)
	go func() { runErr <- bridge.Run(ctx) }()

	// Run が両方向の read を開始できるよう一拍置いてから、ハンドシェイク前に正常切断する。
	time.Sleep(50 * time.Millisecond)
	if err := browserClient.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("close browser: %v", err)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil on normal closure", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not finish after browser close")
	}
}
