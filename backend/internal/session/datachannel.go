package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// dataChannelReadLimit は 1 メッセージあたりの読み取り上限バイト数。ターミナル出力想定でこの上限を超えることはない。
const dataChannelReadLimit = 1 << 20 // 1MiB

// DataChannel は SSM Session Manager / ECS Exec のデータチャネル (agent 側 WebSocket) との接続を表す。
//
// シーケンス番号の管理について: sendSequenceNumber は「ブラウザ→データチャネル」方向の goroutine (キー入力・リサイズ) と
// 「データチャネル→ブラウザ」方向の goroutine (ハンドシェイク応答) の両方から更新されるため、
// 採番と送信を 1 クリティカルセクションで行う sendMu で保護する (採番順と送信順の一致を保証する)。
// expectedSequenceNumber は「データチャネル→ブラウザ」方向の goroutine のみが更新するため mutex 不要。
// また、WebSocket は単一 TCP 接続上の順序保証があり、本実装では再接続・再送を行わないため、
// AWS 公式実装が持つ IncomingMessageBuffer によるアウトオブオーダー処理は実装しない。
//
// 入力のゲートについて: AWS 公式 session-manager-plugin と同様、ハンドシェイク完了前にキー入力や
// リサイズを送信すると agent 側の入力ストリーム処理が乱れるため、SendInput / SendSize は
// handshakeDone が close されるまでブロックする。ハンドシェイク非対応の agent 向けに、
// 通常出力の初回受信でも handshakeDone を close する。
type DataChannel struct {
	conn      *websocket.Conn
	clientID  string
	sessionID string

	sendMu                 sync.Mutex
	sendSequenceNumber     int64
	expectedSequenceNumber int64

	handshakeOnce sync.Once
	handshakeDone chan struct{}
}

// OpenDataChannel は StreamUrl に WebSocket 接続し、OpenDataChannelInput をテキストメッセージとして
// 送信することでハンドシェイクを完了する。
func OpenDataChannel(ctx context.Context, streamURL, tokenValue, sessionID string) (*DataChannel, error) {
	conn, _, err := websocket.Dial(ctx, streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial data channel websocket: %w", err)
	}
	conn.SetReadLimit(dataChannelReadLimit)

	dc := &DataChannel{
		conn:          conn,
		clientID:      uuid.NewString(),
		sessionID:     sessionID,
		handshakeDone: make(chan struct{}),
	}

	input := NewOpenDataChannelInput(dc.clientID, tokenValue)
	payload, err := json.Marshal(input)
	if err != nil {
		// 呼び出し元へ返すエラーが本質のため、close 失敗はログのみに留める。
		if closeErr := conn.Close(websocket.StatusInternalError, "marshal open data channel input failed"); closeErr != nil {
			slog.Warn("failed to close data channel websocket", "err", closeErr)
		}
		return nil, fmt.Errorf("marshal open data channel input: %w", err)
	}

	// AWS 実装 (FinalizeDataChannelHandshake) 同様、ハンドシェイクは TEXT メッセージで送る。
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		if closeErr := conn.Close(websocket.StatusInternalError, "send open data channel input failed"); closeErr != nil {
			slog.Warn("failed to close data channel websocket", "err", closeErr)
		}
		return nil, fmt.Errorf("send open data channel input: %w", err)
	}

	return dc, nil
}

// Close はデータチャネルの WebSocket 接続を閉じる。
func (dc *DataChannel) Close() error {
	return dc.conn.Close(websocket.StatusNormalClosure, "session closed")
}

// SendInput は端末入力バイト列を input_stream_data メッセージとして送信する。
// ハンドシェイク完了までブロックする (ctx キャンセルで解除される)。
func (dc *DataChannel) SendInput(ctx context.Context, payloadType PayloadType, payload []byte) error {
	if err := dc.waitHandshake(ctx); err != nil {
		return err
	}
	return dc.sendInputStreamData(ctx, payloadType, payload)
}

// SendSize は端末サイズ変更を set_size (PayloadType: Size) の input_stream_data メッセージとして送信する。
// ハンドシェイク完了までブロックする (ctx キャンセルで解除される)。
func (dc *DataChannel) SendSize(ctx context.Context, cols, rows uint32) error {
	if err := dc.waitHandshake(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(SizeData{Cols: cols, Rows: rows})
	if err != nil {
		return fmt.Errorf("marshal size data: %w", err)
	}
	return dc.sendInputStreamData(ctx, PayloadTypeSize, payload)
}

// waitHandshake はハンドシェイク完了 (またはハンドシェイク非対応 agent の初回出力受信) まで待機する。
func (dc *DataChannel) waitHandshake(ctx context.Context) error {
	select {
	case <-dc.handshakeDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// markHandshakeDone は入力送信のゲートを解放する。複数回呼んでも安全。
func (dc *DataChannel) markHandshakeDone() {
	dc.handshakeOnce.Do(func() { close(dc.handshakeDone) })
}

// sendInputStreamData は input_stream_data メッセージに送信シーケンス番号を採番して送信する。
// 採番順と実際の送信順を一致させるため、メッセージ生成から送信完了までを sendMu で保護する。
func (dc *DataChannel) sendInputStreamData(ctx context.Context, payloadType PayloadType, payload []byte) error {
	dc.sendMu.Lock()
	defer dc.sendMu.Unlock()
	msg := NewInputStreamDataMessage(dc.sendSequenceNumber, payloadType, payload)
	if err := dc.send(ctx, msg); err != nil {
		return err
	}
	dc.sendSequenceNumber++
	return nil
}

// ReadResult は 1 回の Read で得られた処理結果を表す。
type ReadResult struct {
	// Output は端末に書き出すべきバイト列 (PayloadTypeOutput/StdErr 等)。空の場合は書き出し不要。
	Output []byte
	// Closed は agent 側からセッション終了 (channel_closed) が通知されたことを示す。
	Closed bool
	// CloseMessage は Closed が true の場合の終了メッセージ (空の場合もある)。
	CloseMessage string
}

// Read はデータチャネルから 1 メッセージ受信し、ハンドシェイク応答/acknowledge 送信などのプロトコル処理を行った上で、
// 端末に書き出すべき出力を返す。
func (dc *DataChannel) Read(ctx context.Context) (ReadResult, error) {
	typ, raw, err := dc.conn.Read(ctx)
	if err != nil {
		return ReadResult{}, fmt.Errorf("read data channel message: %w", err)
	}
	if typ != websocket.MessageBinary {
		// agent からの通常メッセージは常に BINARY。TEXT が来るのは想定外だが、致命的ではないので無視する。
		slog.Warn("unexpected websocket message type from data channel", "type", typ)
		return ReadResult{}, nil
	}

	var msg AgentMessage
	if err := msg.Unmarshal(raw); err != nil {
		return ReadResult{}, fmt.Errorf("unmarshal agent message: %w", err)
	}

	switch msg.MessageType {
	case MessageTypeOutputStreamData:
		return dc.handleOutputStreamData(ctx, &msg)
	case MessageTypeAcknowledge:
		// 再送を行わないためバッファ管理は不要。ログのみ残す。
		slog.Debug("received acknowledge message", "sequence_number", msg.SequenceNumber)
		return ReadResult{}, nil
	case MessageTypeChannelClosed:
		var closed struct {
			Output string `json:"Output"`
		}
		if err := json.Unmarshal(msg.Payload, &closed); err != nil {
			slog.Warn("failed to unmarshal channel_closed payload", "err", err)
		}
		return ReadResult{Closed: true, CloseMessage: closed.Output}, nil
	case MessageTypeStartPublication, MessageTypePausePublication:
		return ReadResult{}, nil
	default:
		slog.Warn("unknown message type from data channel", "message_type", msg.MessageType)
		return ReadResult{}, nil
	}
}

// handleOutputStreamData は output_stream_data メッセージを処理する。
// ハンドシェイク系ペイロード (HandshakeRequest/HandshakeComplete) はここで応答し、
// 実際の端末出力 (Output/StdErr 等) のみを呼び出し元に返す。
func (dc *DataChannel) handleOutputStreamData(ctx context.Context, msg *AgentMessage) (ReadResult, error) {
	if msg.SequenceNumber != dc.expectedSequenceNumber {
		// WebSocket は単一 TCP 接続で順序保証されるため、通常は発生しない。
		// 再送・再接続は実装していないため、ここでは処理をスキップして次のメッセージを待つ。
		slog.Warn("unexpected sequence number, skipping message",
			"got", msg.SequenceNumber, "want", dc.expectedSequenceNumber)
		return ReadResult{}, nil
	}

	switch msg.PayloadType {
	case PayloadTypeHandshakeRequest:
		if err := dc.sendAcknowledge(ctx, msg); err != nil {
			return ReadResult{}, err
		}
		if err := dc.handleHandshakeRequest(ctx, msg); err != nil {
			return ReadResult{}, err
		}
	case PayloadTypeHandshakeComplete:
		if err := dc.sendAcknowledge(ctx, msg); err != nil {
			return ReadResult{}, err
		}
		// HandshakeCompletePayload の内容 (CustomerMessage 等) は特に処理せず、開始通知として扱う。
		dc.markHandshakeDone()
	default:
		if err := dc.sendAcknowledge(ctx, msg); err != nil {
			return ReadResult{}, err
		}
		// ハンドシェイク非対応の agent はハンドシェイクなしで出力を送ってくるため、
		// 初回の通常出力でも入力ゲートを解放する。
		dc.markHandshakeDone()
		dc.expectedSequenceNumber++
		return ReadResult{Output: msg.Payload}, nil
	}

	dc.expectedSequenceNumber++
	return ReadResult{}, nil
}

// handleHandshakeRequest は HandshakeRequest を処理し、HandshakeResponse を返送する。
// KMSEncryption アクションは対応しないため常に Unsupported とし、SessionType アクションのみ許可する。
func (dc *DataChannel) handleHandshakeRequest(ctx context.Context, msg *AgentMessage) error {
	var req HandshakeRequestPayload
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return fmt.Errorf("unmarshal handshake request: %w", err)
	}

	resp := HandshakeResponsePayload{
		ClientVersion:          clientVersion,
		ProcessedClientActions: make([]ProcessedClientAction, 0, len(req.RequestedClientActions)),
	}

	var errs []string
	for _, action := range req.RequestedClientActions {
		processed := ProcessedClientAction{ActionType: action.ActionType}
		switch action.ActionType {
		case ActionTypeSessionType:
			var sessReq SessionTypeRequest
			if err := json.Unmarshal(action.ActionParameters, &sessReq); err != nil {
				processed.ActionStatus = ActionStatusFailed
				processed.Error = fmt.Sprintf("unmarshal session type request: %s", err)
				errs = append(errs, processed.Error)
				break
			}
			if !shellSessionTypes[sessReq.SessionType] {
				processed.ActionStatus = ActionStatusFailed
				processed.Error = fmt.Sprintf("unsupported session type %q", sessReq.SessionType)
				errs = append(errs, processed.Error)
				break
			}
			processed.ActionStatus = ActionStatusSuccess
		default:
			// KMSEncryption を含む未対応アクションは全て Unsupported として拒否する。
			processed.ActionStatus = ActionStatusUnsupported
			processed.Error = fmt.Sprintf("unsupported action %q", action.ActionType)
			errs = append(errs, processed.Error)
		}
		resp.ProcessedClientActions = append(resp.ProcessedClientActions, processed)
	}
	resp.Errors = errs

	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal handshake response: %w", err)
	}
	// ハンドシェイク完了前に送る応答のため、入力ゲート (waitHandshake) を通さず直接送信する。
	return dc.sendInputStreamData(ctx, PayloadTypeHandshakeResponse, payload)
}

// sendAcknowledge は受信メッセージへの acknowledge を送信する。
func (dc *DataChannel) sendAcknowledge(ctx context.Context, received *AgentMessage) error {
	ack, err := NewAcknowledgeMessage(received)
	if err != nil {
		return err
	}
	return dc.send(ctx, ack)
}

// send は AgentMessage をシリアライズして BINARY メッセージとして送信する。
func (dc *DataChannel) send(ctx context.Context, msg *AgentMessage) error {
	raw, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("marshal agent message: %w", err)
	}
	if err := dc.conn.Write(ctx, websocket.MessageBinary, raw); err != nil {
		return fmt.Errorf("write data channel message: %w", err)
	}
	return nil
}
