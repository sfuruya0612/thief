// Package session は AWS Systems Manager Session Manager (SSM) のデータチャネルで
// 使われる AgentMessage バイナリフレーミングプロトコルを実装する。
//
// 仕様源は AWS 公式 OSS session-manager-plugin (Apache License 2.0) の
// src/message パッケージ (clientmessage.go / messageparser.go) のフレーム仕様に準拠する。
// https://github.com/aws/session-manager-plugin
package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MessageType は AgentMessage.MessageType フィールドに設定する値。
type MessageType string

// AWS session-manager-plugin の message パッケージが定義するメッセージタイプ。
const (
	MessageTypeInputStreamData  MessageType = "input_stream_data"
	MessageTypeOutputStreamData MessageType = "output_stream_data"
	MessageTypeAcknowledge      MessageType = "acknowledge"
	MessageTypeChannelClosed    MessageType = "channel_closed"
	MessageTypeStartPublication MessageType = "start_publication"
	MessageTypePausePublication MessageType = "pause_publication"
)

// PayloadType は AgentMessage.PayloadType フィールドに設定する値。
type PayloadType uint32

// AWS session-manager-plugin の message パッケージが定義するペイロードタイプ。
const (
	PayloadTypeOutput               PayloadType = 1
	PayloadTypeError                PayloadType = 2
	PayloadTypeSize                 PayloadType = 3
	PayloadTypeParameter            PayloadType = 4
	PayloadTypeHandshakeRequest     PayloadType = 5
	PayloadTypeHandshakeResponse    PayloadType = 6
	PayloadTypeHandshakeComplete    PayloadType = 7
	PayloadTypeEncChallengeRequest  PayloadType = 8
	PayloadTypeEncChallengeResponse PayloadType = 9
	PayloadTypeFlag                 PayloadType = 10
	PayloadTypeStdErr               PayloadType = 11
	PayloadTypeExitCode             PayloadType = 12
)

// AgentMessage のフィールド長 (バイト数)。フィールド順序はワイヤフォーマットの並びと一致する。
const (
	headerLengthFieldLen   = 4
	messageTypeFieldLen    = 32
	schemaVersionFieldLen  = 4
	createdDateFieldLen    = 8
	sequenceNumberFieldLen = 8
	flagsFieldLen          = 8
	messageIDFieldLen      = 16
	payloadDigestFieldLen  = 32
	payloadTypeFieldLen    = 4
	payloadLengthFieldLen  = 4
)

// AgentMessage のフィールドオフセット。
const (
	offsetHeaderLength   = 0
	offsetMessageType    = offsetHeaderLength + headerLengthFieldLen
	offsetSchemaVersion  = offsetMessageType + messageTypeFieldLen
	offsetCreatedDate    = offsetSchemaVersion + schemaVersionFieldLen
	offsetSequenceNumber = offsetCreatedDate + createdDateFieldLen
	offsetFlags          = offsetSequenceNumber + sequenceNumberFieldLen
	offsetMessageID      = offsetFlags + flagsFieldLen
	offsetPayloadDigest  = offsetMessageID + messageIDFieldLen
	offsetPayloadType    = offsetPayloadDigest + payloadDigestFieldLen
	offsetPayloadLength  = offsetPayloadType + payloadTypeFieldLen
	offsetPayload        = offsetPayloadLength + payloadLengthFieldLen
)

// headerLength は AgentMessage.HeaderLength フィールドに設定する固定値。
// ワイヤフォーマット上、HeaderLength フィールド自体と PayloadLength フィールドを除いた
// ヘッダ部分のバイト数 (offsetPayloadLength と同じ値) を表す。
const headerLength = offsetPayloadLength

// Flags のビット。SYN は最初のメッセージであることを示し、FIN は最終メッセージであることを示す。
const (
	FlagSyn uint64 = 1 << 0
	FlagFin uint64 = 1 << 1
)

// AgentMessage は SSM データチャネル上でやり取りされる 1 メッセージを表す。
type AgentMessage struct {
	MessageType    MessageType
	SchemaVersion  uint32
	CreatedDate    uint64 // UnixMilli
	SequenceNumber int64
	Flags          uint64
	MessageID      uuid.UUID
	PayloadType    PayloadType
	Payload        []byte
}

// Marshal は AgentMessage をワイヤフォーマットのバイト列にシリアライズする。
func (m *AgentMessage) Marshal() ([]byte, error) {
	payloadLen := uint32(len(m.Payload))
	buf := make([]byte, offsetPayload+int(payloadLen))

	binary.BigEndian.PutUint32(buf[offsetHeaderLength:], uint32(headerLength))

	if len(m.MessageType) > messageTypeFieldLen {
		return nil, fmt.Errorf("marshal agent message: message type %q exceeds %d bytes", m.MessageType, messageTypeFieldLen)
	}
	// 未使用領域は AWS 実装同様スペースで埋める。
	for i := offsetMessageType; i < offsetSchemaVersion; i++ {
		buf[i] = ' '
	}
	copy(buf[offsetMessageType:], []byte(m.MessageType))

	binary.BigEndian.PutUint32(buf[offsetSchemaVersion:], m.SchemaVersion)
	binary.BigEndian.PutUint64(buf[offsetCreatedDate:], m.CreatedDate)
	binary.BigEndian.PutUint64(buf[offsetSequenceNumber:], uint64(m.SequenceNumber))
	binary.BigEndian.PutUint64(buf[offsetFlags:], m.Flags)

	copy(buf[offsetMessageID:], marshalMessageID(m.MessageID))

	digest := sha256.Sum256(m.Payload)
	copy(buf[offsetPayloadDigest:], digest[:])

	binary.BigEndian.PutUint32(buf[offsetPayloadType:], uint32(m.PayloadType))
	binary.BigEndian.PutUint32(buf[offsetPayloadLength:], payloadLen)
	copy(buf[offsetPayload:], m.Payload)

	return buf, nil
}

// Unmarshal はワイヤフォーマットのバイト列を AgentMessage にデシリアライズする。
// SHA-256 ダイジェストによるペイロード検証も行う。
func (m *AgentMessage) Unmarshal(raw []byte) error {
	if len(raw) < offsetPayload {
		return fmt.Errorf("unmarshal agent message: too short: got %d bytes, want at least %d", len(raw), offsetPayload)
	}

	hl := binary.BigEndian.Uint32(raw[offsetHeaderLength:])
	if int(hl)+payloadLengthFieldLen > len(raw) {
		return fmt.Errorf("unmarshal agent message: header length %d exceeds message size %d", hl, len(raw))
	}

	m.MessageType = MessageType(bytes.TrimSpace(bytes.Trim(raw[offsetMessageType:offsetSchemaVersion], "\x00")))
	m.SchemaVersion = binary.BigEndian.Uint32(raw[offsetSchemaVersion:])
	m.CreatedDate = binary.BigEndian.Uint64(raw[offsetCreatedDate:])
	m.SequenceNumber = int64(binary.BigEndian.Uint64(raw[offsetSequenceNumber:]))
	m.Flags = binary.BigEndian.Uint64(raw[offsetFlags:])
	m.MessageID = unmarshalMessageID(raw[offsetMessageID:offsetPayloadDigest])

	digest := raw[offsetPayloadDigest:offsetPayloadType]
	m.PayloadType = PayloadType(binary.BigEndian.Uint32(raw[offsetPayloadType:]))
	payloadLen := binary.BigEndian.Uint32(raw[offsetPayloadLength:])

	// AWS 公式実装 (DeserializeClientMessage) と同様、PayloadLength フィールドの値では
	// 終端を切らず、入力バイト列の末尾までを Payload として扱う。
	payloadStart := int(hl) + payloadLengthFieldLen
	m.Payload = raw[payloadStart:]

	// 公式実装の Validate() は StartPublicationMessage/PausePublicationMessage について
	// digest 検証自体を行わない (これらのメッセージは PayloadDigest が有効な値を持たない)。
	// データチャネル接続直後に MGS から送られる start_publication がこれに該当するため、
	// 同様にスキップする。
	if payloadLen != 0 && m.MessageType != MessageTypeStartPublication && m.MessageType != MessageTypePausePublication {
		want := sha256.Sum256(m.Payload)
		if !bytes.Equal(want[:], digest) {
			return errors.New("unmarshal agent message: payload digest mismatch")
		}
	}

	return nil
}

// marshalMessageID は標準の UUID バイト列 (先頭 8 バイトが MSB、末尾 8 バイトが LSB) を
// AWS session-manager-plugin のワイヤフォーマット (LSB を先に、MSB を後に置く Java UUID 互換の並び) に変換する。
func marshalMessageID(id uuid.UUID) []byte {
	wire := make([]byte, messageIDFieldLen)
	copy(wire[0:8], id[8:16]) // LSB
	copy(wire[8:16], id[0:8]) // MSB
	return wire
}

// unmarshalMessageID は marshalMessageID の逆変換を行う。
func unmarshalMessageID(wire []byte) uuid.UUID {
	var id uuid.UUID
	copy(id[0:8], wire[8:16]) // MSB
	copy(id[8:16], wire[0:8]) // LSB
	return id
}

// AcknowledgeContent は acknowledge メッセージのペイロード (JSON) を表す。
type AcknowledgeContent struct {
	MessageType         string `json:"AcknowledgedMessageType"`
	MessageID           string `json:"AcknowledgedMessageId"`
	SequenceNumber      int64  `json:"AcknowledgedMessageSequenceNumber"`
	IsSequentialMessage bool   `json:"IsSequentialMessage"`
}

// SizeData は set_size (端末リサイズ) メッセージのペイロード (JSON) を表す。
type SizeData struct {
	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`
}

// ActionType はハンドシェイク中に agent が要求するアクションの種類。
type ActionType string

const (
	ActionTypeKMSEncryption ActionType = "KMSEncryption"
	ActionTypeSessionType   ActionType = "SessionType"
)

// ActionStatus はハンドシェイクアクションの処理結果。
type ActionStatus int

const (
	ActionStatusSuccess     ActionStatus = 1
	ActionStatusFailed      ActionStatus = 2
	ActionStatusUnsupported ActionStatus = 3
)

// HandshakeRequestPayload は agent から送られるハンドシェイク要求 (PayloadType: HandshakeRequest) を表す。
type HandshakeRequestPayload struct {
	AgentVersion           string                  `json:"AgentVersion"`
	RequestedClientActions []RequestedClientAction `json:"RequestedClientActions"`
}

// RequestedClientAction は agent がクライアントに要求する 1 アクション。
type RequestedClientAction struct {
	ActionType       ActionType      `json:"ActionType"`
	ActionParameters json.RawMessage `json:"ActionParameters"`
}

// ProcessedClientAction は RequestedClientAction の処理結果。
type ProcessedClientAction struct {
	ActionType   ActionType   `json:"ActionType"`
	ActionStatus ActionStatus `json:"ActionStatus"`
	ActionResult any          `json:"ActionResult,omitempty"`
	Error        string       `json:"Error,omitempty"`
}

// HandshakeResponsePayload はハンドシェイク要求への応答 (PayloadType: HandshakeResponse) を表す。
type HandshakeResponsePayload struct {
	ClientVersion          string                  `json:"ClientVersion"`
	ProcessedClientActions []ProcessedClientAction `json:"ProcessedClientActions"`
	Errors                 []string                `json:"Errors,omitempty"`
}

// SessionTypeRequest は ActionTypeSessionType の ActionParameters を表す。
type SessionTypeRequest struct {
	SessionType string `json:"SessionType"`
	Properties  any    `json:"Properties"`
}

// HandshakeCompletePayload はハンドシェイク完了通知 (PayloadType: HandshakeComplete) を表す。
type HandshakeCompletePayload struct {
	HandshakeTimeToComplete int64  `json:"HandshakeTimeToComplete"` // ナノ秒単位 (agent 側の time.Duration をそのまま JSON 化した値)
	CustomerMessage         string `json:"CustomerMessage"`
}

// shellSessionTypes は SessionType ハンドシェイクアクションで許容するセッション種別。
// session-manager-plugin の Standard_Stream / InteractiveCommands / NonInteractiveCommands に相当する。
var shellSessionTypes = map[string]bool{
	"Standard_Stream":        true,
	"InteractiveCommands":    true,
	"NonInteractiveCommands": true,
}

// OpenDataChannelInput はデータチャネル接続確立時にテキストメッセージとして送信する
// ハンドシェイクペイロード (JSON) を表す。
type OpenDataChannelInput struct {
	MessageSchemaVersion string `json:"MessageSchemaVersion"`
	RequestID            string `json:"RequestId"`
	TokenValue           string `json:"TokenValue"`
	ClientID             string `json:"ClientId"`
	ClientVersion        string `json:"ClientVersion"`
}

// messageSchemaVersion は OpenDataChannelInput.MessageSchemaVersion に設定する固定値。
const messageSchemaVersion = "1.0"

// clientVersion は OpenDataChannelInput.ClientVersion に設定する固定値。
// session-manager-plugin 本体ではなく自前実装であることを示すため、独自バージョン文字列を用いる。
const clientVersion = "1.0.0"

// NewOpenDataChannelInput はデータチャネルのハンドシェイクペイロードを生成する。
func NewOpenDataChannelInput(clientID, tokenValue string) OpenDataChannelInput {
	return OpenDataChannelInput{
		MessageSchemaVersion: messageSchemaVersion,
		RequestID:            uuid.NewString(),
		TokenValue:           tokenValue,
		ClientID:             clientID,
		ClientVersion:        clientVersion,
	}
}

// NewAcknowledgeMessage は受信した AgentMessage に対する acknowledge メッセージを生成する。
func NewAcknowledgeMessage(received *AgentMessage) (*AgentMessage, error) {
	content := AcknowledgeContent{
		MessageType:         string(received.MessageType),
		MessageID:           received.MessageID.String(),
		SequenceNumber:      received.SequenceNumber,
		IsSequentialMessage: true,
	}
	payload, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal acknowledge content: %w", err)
	}
	return &AgentMessage{
		MessageType:   MessageTypeAcknowledge,
		SchemaVersion: 1,
		CreatedDate:   uint64(time.Now().UnixMilli()),
		MessageID:     uuid.New(),
		Flags:         FlagSyn | FlagFin,
		Payload:       payload,
	}, nil
}

// NewInputStreamDataMessage は入力バイト列 (端末入力) を input_stream_data メッセージに変換する。
// sequenceNumber は呼び出し側 (datachannel) が管理する送信シーケンス番号を渡す。
func NewInputStreamDataMessage(sequenceNumber int64, payloadType PayloadType, payload []byte) *AgentMessage {
	return &AgentMessage{
		MessageType:    MessageTypeInputStreamData,
		SchemaVersion:  1,
		CreatedDate:    uint64(time.Now().UnixMilli()),
		SequenceNumber: sequenceNumber,
		MessageID:      uuid.New(),
		PayloadType:    payloadType,
		Payload:        payload,
	}
}

// NewSizeInputMessage は端末サイズ変更を通知する input_stream_data メッセージ (PayloadType: Size) を生成する。
func NewSizeInputMessage(sequenceNumber int64, cols, rows uint32) (*AgentMessage, error) {
	payload, err := json.Marshal(SizeData{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("marshal size data: %w", err)
	}
	return NewInputStreamDataMessage(sequenceNumber, PayloadTypeSize, payload), nil
}
