# backend 起動前に frontend を開いた場合に接続待ちを表示する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

frontend (ポート 8088) と backend (ポート 8089) は別プロセスで起動する。
backend が起動しきる前に frontend を開くと、API 呼び出しが到達不能となる。
`frontend/src/api/client.ts` は fetch が投げる TypeError を捕捉し、`ApiError(0, 'network_error')` に正規化している。
このとき UI にはリソース取得の失敗として表示され、接続待ちなのか恒久的なエラーなのかをユーザが判別できない。

backend には接続確認専用のエンドポイントが存在しない。
`internal/api/routes.go` に登録されたルートを確認したが、health や readiness に相当するものは無い。

自動復旧の一部は既に実装されている。
`frontend/src/api/queries.ts` の `useProfiles` と `useGcpProjects` は、error 状態のとき `refetchInterval` で `PROFILE_LIST_ERROR_RETRY_INTERVAL` (15 秒) ごとにポーリングし、backend 起動後に自動で成功へ復帰する。
不足しているのは、接続待ちであることをユーザに伝える UI である。

## 目的

backend が未起動または起動途中のとき、frontend が接続待ちであることをユーザに提示する。
既存の自動リトライの上に待機 UI を載せ、backend が起動したら追加操作なしで通常画面へ復帰する。

## 対応内容

### frontend

- 接続待ちであることを示すローディング表示を追加する
- 既存の `refetchInterval` による自動復旧を活かし、疎通が回復したら待機表示を解除する
- 接続待ちの判定は `network_error` を対象とし、SSO 期限切れ (`SSO_TOKEN_EXPIRED`) やその他の業務エラーと区別する

### backend (health 方式を採る場合のみ)

- 接続確認用の軽量なエンドポイント (例: `GET /api/health`) を新規に追加する
- 認証やクラウド呼び出しを伴わず、即座に 200 を返すだけの実装とする

## 設計上の論点

- 接続待ちの検知方式を決める必要がある。既存クエリの `network_error` を監視する方式 (backend 変更なし) と、health エンドポイントを新設して専用に疎通確認する方式のどちらを採るか。health 新設は TODO に無い追加スコープである。
- 接続待ち表示の対象範囲を決める必要がある。起動時の初回疎通に限定するか、セッション途中の一時的な `network_error` (瞬断) も対象にするか。後者を含めると通常運用中の単発エラーで現在のビューを失う副作用があるため、保護条件を設ける。
- health 方式を採る場合の検知間隔 (ポーリング周期) と、TanStack Query の `retry` / `retryDelay` の設定値を決める必要がある。

## 完了条件

- backend 未起動の状態で frontend を開くと接続待ち表示が出る
- backend を起動すると追加操作なしで通常画面へ復帰する
- backend 起動済みの通常時に接続待ち表示が出ない

## 解決方法

検知方式は health エンドポイント新設を採用し、対象範囲は起動時の初回疎通のみとした
(ユーザ判断)。

### backend

- `GET /api/health` を新設した (`internal/api/handlers_health.go`)。認証やクラウド呼び出しを
  伴わず、`{"status": "ok"}` を即座に返すだけの実装。

### frontend

- `api/queries.ts` に `useHealthCheck` を追加した。`staleTime: Infinity` かつ
  `refetchInterval` は `error` のときのみ 2 秒間隔で再取得する。成功後は再取得されなくなるため、
  対象は起動時の初回疎通のみに限定され、セッション途中の一時的な `network_error` では待機表示に
  戻らない。
- `components/ConnectionWaiting.tsx` を新設し、`App.tsx` で `useHealthCheck` が成功するまで
  画面全体をこのコンポーネントに差し替える。

### 完了条件の確認

- backend を起動せず `GET /api/health` を呼ぶと到達不能 (接続待ち表示が出る側の入力) になることを
  fetch 到達不能時の `ApiError(0, 'network_error')` 経路で確認した。
- `go run ./cmd/thief server` を起動した状態で `GET /api/health` が 200 を返すことを確認した。
