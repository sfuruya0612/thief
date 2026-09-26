# EC2/ECS ターミナルが間欠的にキー入力を受け付けなくなる

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 症状

EC2 Start Session / ECS Exec のブラウザターミナルで、セッション自体は開始してプロンプトなどの出力は表示されるのに、キー入力が一切反映されない状態に間欠的に陥る。発生はタイミング依存で、同じ操作でも成功することがある。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` でアプリを起動する
2. EC2 インスタンスの Start Session または ECS タスクの Exec でターミナルを開く
3. ターミナルの開閉を繰り返す (10 回程度)
4. 数回に 1 回の頻度で、プロンプトは表示されるがキー入力が画面に反映されないセッションが発生する

発生時、Drawer を閉じてブラウザ側が切断すると backend に以下のログが出る (これは切断の結果であり原因ではない)。

```
ERROR session bridge ended with error err="session bridge: failed to get reader: received close frame: status = StatusGoingAway and reason = \"\""
```

## 原因

`backend/internal/session/datachannel.go` の送信シーケンス番号 `sendSequenceNumber` にデータ競合がある。

ドキュメントコメント (datachannel.go:18-22) は「`sendSequenceNumber` はブラウザ→データチャネル方向の goroutine のみが更新する」前提で mutex を持たないと宣言しているが、実際には 2 つの goroutine が更新している。

1. データチャネル→ブラウザ方向の goroutine: `handleHandshakeRequest` が `SendInput(PayloadTypeHandshakeResponse, ...)` を呼ぶ (datachannel.go:230)
2. ブラウザ→データチャネル方向の goroutine: キー入力の `SendInput` (bridge.go:138) とリサイズの `SendSize` (bridge.go:159)

フロントエンド (`frontend/src/components/Terminal/Terminal.tsx`) は WebSocket の `onopen` で即座に resize 制御メッセージを送るため、SSM ハンドシェイク応答の送信とほぼ同時刻に `SendSize` が実行される。`SendInput`/`SendSize` の「メッセージ生成 → 送信 → インクリメント」は非アトミックな read-modify-write であり、衝突すると以下のいずれかが起きる。

- 同一シーケンス番号で 2 つの input_stream_data が送信され、agent 側で重複として破棄される
- カウンタがスキップし、agent は欠番のシーケンスを待ち続けて以降の入力を永遠にバッファする

出力方向のカウンタ (`expectedSequenceNumber`) は独立しているため出力は正常に表示され、「出力は見えるが入力が効かない」という症状に一致する。

## 副次的な問題

- AWS 公式 session-manager-plugin はハンドシェイク完了まで入力を送信しないが、本実装は `onopen` 直後の resize やキー入力がハンドシェイク中に割り込める
- `NewInputStreamDataMessage` (protocol.go:327-337) が常に `Flags: 0` であり、AWS 公式実装がシーケンス番号 0 のストリームデータに立てる SYN フラグがない
- ブラウザの正常切断 (`StatusGoingAway`/`StatusNormalClosure`) が ERROR レベルでログ出力され、トリアージを誤らせる

## 解決方法

`backend/internal/session/` の以下を修正した。

- `datachannel.go`: 送信シーケンス番号の採番と WebSocket 送信を 1 クリティカルセクションで行う内部メソッド `sendInputStreamData` を新設し、`sendMu` (sync.Mutex) で保護した。採番順と送信順の一致を保証し、重複・欠番を排除した。
- `datachannel.go`: `handshakeDone chan struct{}` + `sync.Once` による入力ゲートを追加した。公開 `SendInput` / `SendSize` はハンドシェイク完了 (HandshakeComplete 受信、またはハンドシェイク非対応 agent の初回通常出力受信) までブロックする。ctx キャンセルで確実に解除される。ハンドシェイク応答自体はゲートを通らず送信する。
- `protocol.go`: `NewInputStreamDataMessage` でシーケンス番号 0 のメッセージに SYN フラグを立てるようにした (AWS 公式 session-manager-plugin の SendStreamDataMessage と同挙動)。専用コンストラクタ `NewSizeInputMessage` は SendSize 側にペイロード生成を移して削除した。
- `bridge.go`: `pumpBrowserToDataChannel` でブラウザの正常切断 (`StatusNormalClosure` / `StatusGoingAway`) をエラーではなくブリッジの正常終了として扱い、ERROR ログを出さないようにした。

テストとして `datachannel_test.go` / `bridge_test.go` を新規追加した (フェイク agent / ブラウザ WebSocket による E2E 検証、並行送信のシーケンス番号検証、`go test -race` での競合検出を含む)。`mise run check` 全通過を確認した。
