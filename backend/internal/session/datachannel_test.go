package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testTimeout はテスト全体のデッドライン。ハングした場合にテストを確実に失敗させる。
const testTimeout = 10 * time.Second

// fakeAgent は SSM agent 側の WebSocket エンドポイントをエミュレートするテストサーバ。
type fakeAgent struct {
	server *httptest.Server
	conns  chan *websocket.Conn
}

func newFakeAgent(t *testing.T) *fakeAgent {
	t.Helper()
	fa := &fakeAgent{conns: make(chan *websocket.Conn, 1)}
	fa.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept fake agent websocket: %v", err)
			return
		}
		conn.SetReadLimit(dataChannelReadLimit)
		fa.conns <- conn
	}))
	t.Cleanup(fa.server.Close)
	return fa
}

// accept はクライアント (DataChannel) からの接続を待って agent 側 conn を返す。
func (fa *fakeAgent) accept(ctx context.Context, t *testing.T) *websocket.Conn {
	t.Helper()
	select {
	case conn := <-fa.conns:
		t.Cleanup(func() {
			// テスト側で明示的に閉じた場合の二重クローズエラーは無視する。
			_ = conn.Close(websocket.StatusNormalClosure, "test done")
		})
		return conn
	case <-ctx.Done():
		t.Fatal("timed out waiting for agent connection")
		return nil
	}
}

// openTestDataChannel はフェイク agent への DataChannel と agent 側 conn を確立して返す。
// 接続直後の OpenDataChannelInput (TEXT) の検証・読み捨てまで行う。
func openTestDataChannel(ctx context.Context, t *testing.T) (*DataChannel, *websocket.Conn) {
	t.Helper()
	fa := newFakeAgent(t)
	dc, err := OpenDataChannel(ctx, fa.server.URL, "test-token", "test-session-id")
	if err != nil {
		t.Fatalf("OpenDataChannel() error = %v", err)
	}
	t.Cleanup(func() { _ = dc.Close() })

	agent := fa.accept(ctx, t)

	typ, raw, err := agent.Read(ctx)
	if err != nil {
		t.Fatalf("read open data channel input: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("open data channel input type = %v, want %v", typ, websocket.MessageText)
	}
	var input OpenDataChannelInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatalf("unmarshal open data channel input: %v", err)
	}
	if input.TokenValue != "test-token" {
		t.Fatalf("TokenValue = %q, want %q", input.TokenValue, "test-token")
	}
	return dc, agent
}

// startReadLoop は bridge の pumpDataChannelToBrowser 相当の read ループを起動し、
// 端末出力 (ReadResult.Output) を返す channel を返す。
func startReadLoop(ctx context.Context, dc *DataChannel) <-chan []byte {
	outputs := make(chan []byte, 16)
	go func() {
		defer close(outputs)
		for {
			result, err := dc.Read(ctx)
			if err != nil {
				return
			}
			if len(result.Output) > 0 {
				outputs <- result.Output
			}
			if result.Closed {
				return
			}
		}
	}()
	return outputs
}

// sendOutputStreamData は agent → client の output_stream_data を送信する。
func sendOutputStreamData(ctx context.Context, t *testing.T, agent *websocket.Conn, seq int64, payloadType PayloadType, payload []byte) {
	t.Helper()
	msg := &AgentMessage{
		MessageType:    MessageTypeOutputStreamData,
		SchemaVersion:  1,
		CreatedDate:    uint64(time.Now().UnixMilli()),
		SequenceNumber: seq,
		PayloadType:    payloadType,
		Payload:        payload,
	}
	raw, err := msg.Marshal()
	if err != nil {
		t.Fatalf("marshal output stream data: %v", err)
	}
	if err := agent.Write(ctx, websocket.MessageBinary, raw); err != nil {
		t.Fatalf("write output stream data: %v", err)
	}
}

// sendHandshakeRequest は agent → client の HandshakeRequest (SessionType: Standard_Stream) を送信する。
func sendHandshakeRequest(ctx context.Context, t *testing.T, agent *websocket.Conn, seq int64) {
	t.Helper()
	req := HandshakeRequestPayload{
		AgentVersion: "3.0.0.0",
		RequestedClientActions: []RequestedClientAction{
			{ActionType: ActionTypeSessionType, ActionParameters: json.RawMessage(`{"SessionType":"Standard_Stream","Properties":null}`)},
		},
	}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal handshake request: %v", err)
	}
	sendOutputStreamData(ctx, t, agent, seq, PayloadTypeHandshakeRequest, payload)
}

// sendHandshakeComplete は agent → client の HandshakeComplete を送信する。
func sendHandshakeComplete(ctx context.Context, t *testing.T, agent *websocket.Conn, seq int64) {
	t.Helper()
	payload, err := json.Marshal(HandshakeCompletePayload{CustomerMessage: ""})
	if err != nil {
		t.Fatalf("marshal handshake complete: %v", err)
	}
	sendOutputStreamData(ctx, t, agent, seq, PayloadTypeHandshakeComplete, payload)
}

// readInputStreamData は client から届くメッセージのうち acknowledge を読み飛ばし、
// 次の input_stream_data を返す。
func readInputStreamData(ctx context.Context, t *testing.T, agent *websocket.Conn) *AgentMessage {
	t.Helper()
	for {
		typ, raw, err := agent.Read(ctx)
		if err != nil {
			t.Fatalf("read message from client: %v", err)
		}
		if typ != websocket.MessageBinary {
			t.Fatalf("message type from client = %v, want %v", typ, websocket.MessageBinary)
		}
		var msg AgentMessage
		if err := msg.Unmarshal(raw); err != nil {
			t.Fatalf("unmarshal message from client: %v", err)
		}
		switch msg.MessageType {
		case MessageTypeAcknowledge:
			continue
		case MessageTypeInputStreamData:
			return &msg
		default:
			t.Fatalf("unexpected message type from client: %q", msg.MessageType)
		}
	}
}

// completeHandshake は agent 側からハンドシェイクを実施し、client の HandshakeResponse を検証して返す。
func completeHandshake(ctx context.Context, t *testing.T, agent *websocket.Conn) *AgentMessage {
	t.Helper()
	sendHandshakeRequest(ctx, t, agent, 0)
	resp := readInputStreamData(ctx, t, agent)
	if resp.PayloadType != PayloadTypeHandshakeResponse {
		t.Fatalf("first input_stream_data payload type = %d, want %d (handshake response)", resp.PayloadType, PayloadTypeHandshakeResponse)
	}
	sendHandshakeComplete(ctx, t, agent, 1)
	return resp
}

func TestDataChannelHandshakeAndInputGate(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dc, agent := openTestDataChannel(ctx, t)
	startReadLoop(ctx, dc)

	// ハンドシェイク完了前に入力とリサイズを送り始める (ws.onopen 直後の resize を再現)。
	// ゲートによりブロックされ、HandshakeResponse より先に agent へ届かないことを検証する。
	sendErrs := make(chan error, 2)
	go func() { sendErrs <- dc.SendSize(ctx, 120, 40) }()
	go func() { sendErrs <- dc.SendInput(ctx, PayloadTypeOutput, []byte("ls\n")) }()

	resp := completeHandshake(ctx, t, agent)
	if resp.SequenceNumber != 0 {
		t.Errorf("handshake response sequence number = %d, want 0", resp.SequenceNumber)
	}
	if resp.Flags != FlagSyn {
		t.Errorf("handshake response flags = %d, want %d (SYN)", resp.Flags, FlagSyn)
	}
	var payload HandshakeResponsePayload
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal handshake response payload: %v", err)
	}
	if len(payload.ProcessedClientActions) != 1 || payload.ProcessedClientActions[0].ActionStatus != ActionStatusSuccess {
		t.Errorf("ProcessedClientActions = %+v, want single success", payload.ProcessedClientActions)
	}

	for range 2 {
		if err := <-sendErrs; err != nil {
			t.Fatalf("send after handshake error = %v", err)
		}
	}

	// ゲート解放後の 2 メッセージが seq 1, 2 で重複・欠番なく届くこと。
	gotSeqs := map[int64]PayloadType{}
	for range 2 {
		msg := readInputStreamData(ctx, t, agent)
		if msg.Flags != 0 {
			t.Errorf("input flags = %d, want 0 (seq %d)", msg.Flags, msg.SequenceNumber)
		}
		if _, dup := gotSeqs[msg.SequenceNumber]; dup {
			t.Fatalf("duplicate sequence number %d", msg.SequenceNumber)
		}
		gotSeqs[msg.SequenceNumber] = msg.PayloadType
	}
	if _, ok := gotSeqs[1]; !ok {
		t.Errorf("sequence numbers = %v, want to contain 1", gotSeqs)
	}
	if _, ok := gotSeqs[2]; !ok {
		t.Errorf("sequence numbers = %v, want to contain 2", gotSeqs)
	}
}

func TestDataChannelConcurrentSendsKeepSequenceContiguous(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dc, agent := openTestDataChannel(ctx, t)
	startReadLoop(ctx, dc)

	// ハンドシェイク応答 (read goroutine) と入力・リサイズ (複数 goroutine) を意図的に競合させ、
	// go test -race での競合検出と、シーケンス番号の重複・欠番がないことを検証する。
	const inputSends = 8
	var wg sync.WaitGroup
	for range inputSends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := dc.SendInput(ctx, PayloadTypeOutput, []byte("x")); err != nil {
				t.Errorf("SendInput() error = %v", err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := dc.SendSize(ctx, 80, 24); err != nil {
			t.Errorf("SendSize() error = %v", err)
		}
	}()

	resp := completeHandshake(ctx, t, agent)
	if resp.SequenceNumber != 0 {
		t.Errorf("handshake response sequence number = %d, want 0", resp.SequenceNumber)
	}
	wg.Wait()

	const totalSends = inputSends + 1
	seen := map[int64]bool{}
	for range totalSends {
		msg := readInputStreamData(ctx, t, agent)
		if seen[msg.SequenceNumber] {
			t.Fatalf("duplicate sequence number %d", msg.SequenceNumber)
		}
		seen[msg.SequenceNumber] = true
	}
	for seq := int64(1); seq <= totalSends; seq++ {
		if !seen[seq] {
			t.Errorf("missing sequence number %d (got %v)", seq, seen)
		}
	}
}

func TestDataChannelSendUnblocksOnContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dc, _ := openTestDataChannel(ctx, t)

	// ハンドシェイクが永遠に完了しない場合でも ctx キャンセルで待機が解除されること。
	sendCtx, sendCancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- dc.SendInput(sendCtx, PayloadTypeOutput, []byte("x")) }()
	sendCancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SendInput() error = %v, want context.Canceled", err)
		}
	case <-ctx.Done():
		t.Fatal("SendInput did not unblock after context cancel")
	}
}

func TestDataChannelInputGateOpensOnPlainOutput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	dc, agent := openTestDataChannel(ctx, t)
	outputs := startReadLoop(ctx, dc)

	// ハンドシェイク非対応の agent がいきなり通常出力を送ってきた場合も入力ゲートが解放されること。
	sendOutputStreamData(ctx, t, agent, 0, PayloadTypeOutput, []byte("$ "))

	select {
	case out := <-outputs:
		if string(out) != "$ " {
			t.Fatalf("output = %q, want %q", out, "$ ")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for output")
	}

	if err := dc.SendInput(ctx, PayloadTypeOutput, []byte("ls\n")); err != nil {
		t.Fatalf("SendInput() error = %v", err)
	}
	msg := readInputStreamData(ctx, t, agent)
	if msg.SequenceNumber != 0 {
		t.Errorf("sequence number = %d, want 0", msg.SequenceNumber)
	}
	if msg.Flags != FlagSyn {
		t.Errorf("flags = %d, want %d (SYN)", msg.Flags, FlagSyn)
	}
	if string(msg.Payload) != "ls\n" {
		t.Errorf("payload = %q, want %q", msg.Payload, "ls\n")
	}
}
