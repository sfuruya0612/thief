package cli

import (
	"errors"
	"os/exec"

	"github.com/sfuruya0612/thief/backend/internal/util"
)

// sessionManagerSessionJSON は session-manager-plugin が期待する StartSession API
// レスポンス形状 (SessionId / StreamUrl / TokenValue) の JSON を組み立てる。
// フィールド名はプラグインとの契約であり変更してはならない。
func sessionManagerSessionJSON(sessionID, streamURL, tokenValue string) ([]byte, error) {
	return util.Parser(struct {
		SessionId  string
		StreamUrl  string
		TokenValue string
	}{
		SessionId:  sessionID,
		StreamUrl:  streamURL,
		TokenValue: tokenValue,
	})
}

// lookupSessionManagerPlugin は PATH から session-manager-plugin の実行ファイルを解決する。
func lookupSessionManagerPlugin() (string, error) {
	plug, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return "", errors.New("session-manager-plugin not found in PATH")
	}
	return plug, nil
}
