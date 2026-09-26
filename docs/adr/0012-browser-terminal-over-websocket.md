# 0012. ブラウザのターミナルは Go で SSM のデータチャネルを中継し、常駐のドックに置く

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

EC2 の Session Manager と ECS Exec に入るには、通常は AWS CLI と Session Manager プラグインが要る。
thief は Web UI では、ブラウザだけで接続させたい。
また、当初はターミナルをサービスの Drawer の中に置いていたため、画面を切り替えるとターミナルがアンマウントされ、`ws.close()` から backend の `TerminateSSMSession` が呼ばれてセッションが終わっていた (issue 0174)。

## 決定

- backend が SSM のデータチャネルのプロトコルを実装し (`backend/internal/session/`)、ブラウザとの間を WebSocket (`github.com/coder/websocket`) で中継する (`session.Bridge.Run`、最初の実装のコミット 2aac22f、2026-07-08 から)。
- どちらかの方向の接続が終わったら、データチャネルを閉じ、`TerminateSSMSession` でセッションを終える。
  終了の待ち時間 5 秒は `backend/internal/aws/ssm_session.go` の公開定数 `TerminateSessionGracePeriod` の 1 か所に置く (issue 0136)。
- frontend はターミナルを `App` の直下のドック (`frontend/src/components/Terminal/TerminalDock.tsx`) に常駐させる。
  非アクティブなターミナルは `hidden` で隠し、アンマウントしない。
  セッションを終える操作は、ターミナルのタブを閉じることだけとする (issue 0174、2026-09-16)。
- backend にはセッションの登録簿を設けない。
- CLI の `thief ec2 session` と `thief ecs exec` は、この中継を使わず、PATH にある `session-manager-plugin` を起動する (`backend/internal/cli/session_plugin.go`)。

## 検討した代替案

- issue 0174: backend にセッションの登録簿を設け、再接続できるようにする。
  孤立したセッションの期限と出力のバッファの管理が要り、再読み込みをまたぐ維持は要件に無いため採らなかった。
- issue 0136: 待ち時間を内部で持つ `TerminateSSMSessionWithGrace` を新設する (案 B)。
  context を第一引数で受ける方針から外れるため採らなかった。
  各所に非公開の定数を置いたまま一致をテストする (案 C)。
  非公開の定数はテストできないため成立しなかった。
- Web UI でも `session-manager-plugin` をサブプロセスで使う案を比べた記録は無い。

## 結果

- Web UI から接続する利用者は、`session-manager-plugin` を用意しなくてよい。
  CLI から接続する利用者は用意する必要がある。
- SSM のデータチャネルのプロトコルの保守を thief が負う。
  プロトコルの仕様は AWS 公式の `session-manager-plugin` (Apache License 2.0) の実装に従う (`backend/internal/session/protocol.go`)。
- ブラウザを再読み込みすると、開いていたセッションは終わる。
- `ReadTimeout` と `WriteTimeout` を設定できない (ADR 0011)。

## 根拠資料

- `docs/issues/closed/0136`、`0174`、`0176`
- `backend/internal/session/bridge.go`、`backend/internal/api/handlers_session.go`
- `frontend/src/components/Terminal/TerminalDock.tsx`、`frontend/src/hooks/useTerminalSessions.ts`
