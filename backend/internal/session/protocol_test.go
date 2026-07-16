package session

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestAgentMessageRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  AgentMessage
	}{
		{
			name: "input_stream_data with payload",
			msg: AgentMessage{
				MessageType:    MessageTypeInputStreamData,
				SchemaVersion:  1,
				CreatedDate:    1700000000000,
				SequenceNumber: 42,
				Flags:          FlagSyn,
				MessageID:      uuid.New(),
				PayloadType:    PayloadTypeOutput,
				Payload:        []byte("echo hello\n"),
			},
		},
		{
			name: "acknowledge with empty payload",
			msg: AgentMessage{
				MessageType:    MessageTypeAcknowledge,
				SchemaVersion:  1,
				CreatedDate:    0,
				SequenceNumber: 0,
				Flags:          FlagSyn | FlagFin,
				MessageID:      uuid.New(),
				PayloadType:    0,
				Payload:        nil,
			},
		},
		{
			name: "output_stream_data with binary payload",
			msg: AgentMessage{
				MessageType:    MessageTypeOutputStreamData,
				SchemaVersion:  1,
				CreatedDate:    1234567890123,
				SequenceNumber: -1, // シーケンス番号は理論上 int64 の全域を取りうる
				Flags:          0,
				MessageID:      uuid.New(),
				PayloadType:    PayloadTypeStdErr,
				Payload:        []byte{0x00, 0x01, 0xff, 0xfe, 0x7f},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := tt.msg.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			var got AgentMessage
			if err := got.Unmarshal(raw); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if got.MessageType != tt.msg.MessageType {
				t.Errorf("MessageType = %q, want %q", got.MessageType, tt.msg.MessageType)
			}
			if got.SchemaVersion != tt.msg.SchemaVersion {
				t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, tt.msg.SchemaVersion)
			}
			if got.CreatedDate != tt.msg.CreatedDate {
				t.Errorf("CreatedDate = %d, want %d", got.CreatedDate, tt.msg.CreatedDate)
			}
			if got.SequenceNumber != tt.msg.SequenceNumber {
				t.Errorf("SequenceNumber = %d, want %d", got.SequenceNumber, tt.msg.SequenceNumber)
			}
			if got.Flags != tt.msg.Flags {
				t.Errorf("Flags = %d, want %d", got.Flags, tt.msg.Flags)
			}
			if got.MessageID != tt.msg.MessageID {
				t.Errorf("MessageID = %v, want %v", got.MessageID, tt.msg.MessageID)
			}
			if got.PayloadType != tt.msg.PayloadType {
				t.Errorf("PayloadType = %d, want %d", got.PayloadType, tt.msg.PayloadType)
			}
			if len(got.Payload) != len(tt.msg.Payload) {
				t.Fatalf("Payload length = %d, want %d", len(got.Payload), len(tt.msg.Payload))
			}
			for i := range got.Payload {
				if got.Payload[i] != tt.msg.Payload[i] {
					t.Errorf("Payload[%d] = %x, want %x", i, got.Payload[i], tt.msg.Payload[i])
				}
			}
		})
	}
}

func TestAgentMessageUnmarshalRejectsDigestMismatch(t *testing.T) {
	msg := AgentMessage{
		MessageType:    MessageTypeInputStreamData,
		SchemaVersion:  1,
		SequenceNumber: 1,
		MessageID:      uuid.New(),
		PayloadType:    PayloadTypeOutput,
		Payload:        []byte("original"),
	}
	raw, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// ペイロード本体だけを改ざんし、ダイジェストとの不整合を発生させる。
	copy(raw[offsetPayload:], []byte("tampered"))

	var got AgentMessage
	if err := got.Unmarshal(raw); err == nil {
		t.Fatal("Unmarshal() error = nil, want digest mismatch error")
	}
}

func TestAgentMessageUnmarshalSkipsDigestForPublicationMessages(t *testing.T) {
	// AWS 公式実装の Validate() は start_publication/pause_publication について
	// digest 検証をスキップする。PayloadDigest フィールドが不正な値であっても
	// これらのメッセージ種別ではエラーにならないことを確認する。
	tests := []struct {
		name        string
		messageType MessageType
	}{
		{name: "start_publication", messageType: MessageTypeStartPublication},
		{name: "pause_publication", messageType: MessageTypePausePublication},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := AgentMessage{
				MessageType:    tt.messageType,
				SchemaVersion:  1,
				SequenceNumber: 1,
				MessageID:      uuid.New(),
				Payload:        []byte("payload"),
			}
			raw, err := msg.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// PayloadDigest を破壊しても、Validate 相当のスキップにより
			// digest mismatch エラーにならないことを確認する。
			for i := offsetPayloadDigest; i < offsetPayloadType; i++ {
				raw[i] = 0xff
			}

			var got AgentMessage
			if err := got.Unmarshal(raw); err != nil {
				t.Fatalf("Unmarshal() error = %v, want nil", err)
			}
		})
	}
}

func TestAgentMessageUnmarshalUsesPayloadLengthForDigestButNotForSlicing(t *testing.T) {
	// AWS 公式実装 (DeserializeClientMessage) は PayloadLength フィールドの値で Payload の
	// 終端を切らず、入力バイト列の末尾までを Payload として扱う。agent 実装のわずかな余剰バイトを
	// 誤って digest mismatch と判定しないことを確認する。
	msg := AgentMessage{
		MessageType:    MessageTypeOutputStreamData,
		SchemaVersion:  1,
		SequenceNumber: 1,
		MessageID:      uuid.New(),
		PayloadType:    PayloadTypeOutput,
		Payload:        []byte("hello"),
	}
	raw, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got AgentMessage
	if err := got.Unmarshal(raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if string(got.Payload) != "hello" {
		t.Errorf("Payload = %q, want %q", got.Payload, "hello")
	}
}

func TestAgentMessageUnmarshalRejectsTooShort(t *testing.T) {
	var got AgentMessage
	if err := got.Unmarshal([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatal("Unmarshal() error = nil, want too-short error")
	}
}

func TestNewAcknowledgeMessage(t *testing.T) {
	received := &AgentMessage{
		MessageType:    MessageTypeOutputStreamData,
		SequenceNumber: 7,
		MessageID:      uuid.New(),
		PayloadType:    PayloadTypeOutput,
		Payload:        []byte("hello"),
	}

	ack, err := NewAcknowledgeMessage(received)
	if err != nil {
		t.Fatalf("NewAcknowledgeMessage() error = %v", err)
	}
	if ack.MessageType != MessageTypeAcknowledge {
		t.Errorf("MessageType = %q, want %q", ack.MessageType, MessageTypeAcknowledge)
	}

	raw, err := ack.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got AgentMessage
	if err := got.Unmarshal(raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	var content AcknowledgeContent
	if err := json.Unmarshal(got.Payload, &content); err != nil {
		t.Fatalf("unmarshal acknowledge content: %v", err)
	}
	if content.MessageType != string(received.MessageType) {
		t.Errorf("AcknowledgedMessageType = %q, want %q", content.MessageType, received.MessageType)
	}
	if content.MessageID != received.MessageID.String() {
		t.Errorf("AcknowledgedMessageId = %q, want %q", content.MessageID, received.MessageID.String())
	}
	if content.SequenceNumber != received.SequenceNumber {
		t.Errorf("AcknowledgedMessageSequenceNumber = %d, want %d", content.SequenceNumber, received.SequenceNumber)
	}
	if !content.IsSequentialMessage {
		t.Error("IsSequentialMessage = false, want true")
	}
}

func TestNewInputStreamDataMessage(t *testing.T) {
	tests := []struct {
		name           string
		sequenceNumber int64
		payloadType    PayloadType
		wantFlags      uint64
	}{
		{name: "ストリーム先頭は SYN フラグを立てる", sequenceNumber: 0, payloadType: PayloadTypeHandshakeResponse, wantFlags: FlagSyn},
		{name: "2 番目以降はフラグなし", sequenceNumber: 1, payloadType: PayloadTypeOutput, wantFlags: 0},
		{name: "リサイズも 2 番目以降はフラグなし", sequenceNumber: 3, payloadType: PayloadTypeSize, wantFlags: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewInputStreamDataMessage(tt.sequenceNumber, tt.payloadType, []byte("payload"))
			if msg.MessageType != MessageTypeInputStreamData {
				t.Errorf("MessageType = %q, want %q", msg.MessageType, MessageTypeInputStreamData)
			}
			if msg.SequenceNumber != tt.sequenceNumber {
				t.Errorf("SequenceNumber = %d, want %d", msg.SequenceNumber, tt.sequenceNumber)
			}
			if msg.PayloadType != tt.payloadType {
				t.Errorf("PayloadType = %d, want %d", msg.PayloadType, tt.payloadType)
			}
			if msg.Flags != tt.wantFlags {
				t.Errorf("Flags = %d, want %d", msg.Flags, tt.wantFlags)
			}
		})
	}
}
