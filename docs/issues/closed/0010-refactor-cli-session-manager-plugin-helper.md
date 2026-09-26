# session-manager-plugin 起動の定型処理を ec2 / ecs コマンド間で共通化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/cli/ec2.go` の `startEC2Session` (140-163 行) と `backend/internal/cli/ecs.go` の `ecsExecuteCommand` (216-240 行) が、以下を同一コードで実装している。

1. session-manager-plugin が期待する Session JSON のマーシャル (完全一致):

```go
sessJSON, err := util.Parser(struct {
    SessionId  string
    StreamUrl  string
    TokenValue string
}{
    SessionId:  session.SessionID,
    StreamUrl:  session.StreamURL,
    TokenValue: session.TokenValue,
})
```

2. プラグイン探索 (完全一致):

```go
plug, err := exec.LookPath("session-manager-plugin")
if err != nil {
    return errors.New("session-manager-plugin not found in PATH")
}
```

セッション接続系コマンドを追加するたびに複製される定型であり、Session JSON のフィールド名 (session-manager-plugin との契約) が 2 箇所に散らばるのは危険。

## 対応方針

cli パッケージにヘルパを追加する。

- `sessionManagerSessionJSON(sessionID, streamURL, tokenValue string) ([]byte, error)`
- `lookupSessionManagerPlugin() (string, error)`

exec エラー後の後処理 (EC2 側の `TerminateSSMSession` 呼び出し) とエラーメッセージの文言はそれぞれ呼び出し側に残し、挙動を変えない。

## CLI 出力 / API レスポンス / 画面表示への影響

なし。組み立てられる JSON・エラーメッセージ・実行コマンドは同一。

## 解決方法

- `backend/internal/cli/session_plugin.go` を新規追加し、以下の 2 ヘルパを定義した。
  - `sessionManagerSessionJSON(sessionID, streamURL, tokenValue string) ([]byte, error)`: session-manager-plugin が期待する Session JSON (SessionId / StreamUrl / TokenValue) の組み立て。フィールド名がプラグインとの契約である旨をコメントで明記。
  - `lookupSessionManagerPlugin() (string, error)`: PATH からのプラグイン探索と "session-manager-plugin not found in PATH" エラーの生成。
- `cli/ec2.go` の `startEC2Session` と `cli/ecs.go` の `ecsExecuteCommand` の重複ブロックをヘルパ呼び出しに置換した。マーシャルエラーのラップ文言・exec エラー時の後処理 (EC2 側の TerminateSSMSession) は呼び出し側に残し、挙動を変えていない。
- `mise run check` 全通過を確認した。
