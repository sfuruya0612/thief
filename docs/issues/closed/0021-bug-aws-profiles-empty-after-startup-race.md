# 0021 backend より先にページを開くと AWS プロファイル一覧が 0 件のまま復旧しない

Created: 2026-07-17
Completed: 2026-07-17
Model: Claude Fable 5 claude-fable-5

## 症状

frontend (vite dev server) を backend (`thief server`) より先に起動し、backend が listen する前にブラウザでページを開くと、AWS のプロファイル一覧が 0 件のままになる。セッションタブのピッカーは「~/.aws/config · 0件」と表示し、旧 localStorage からの移行タブが無い場合はタブバーが空 + SessionEmptyState になる。backend が起動した後も自動では復旧せず、エラーも UI に表示されない。

Google Cloud のプロジェクト一覧も同じ競合でエラーになるが、GCP はピッカーに refresh ボタン (`useRefreshGcpProjects` → invalidateQueries) があるため手動で復旧できる。AWS には同等の復旧手段が無い。

## 再現手順

1. backend を停止した状態で `mise run frontend:run` を実行する
2. ブラウザで http://localhost:8082 を開き、AWS ビューを表示する (この時点で `/api/aws/profiles` への fetch が connection refused で失敗する)
3. その後 `mise run backend:run` (または `thief server`) で backend を起動する
4. ブラウザを操作しても AWS プロファイル一覧は 0 件のまま変化しない (curl では 200 で全件返る)

実際の発生例: 2026-07-17 に vite が 17:41:16、`thief server` が 17:41:26 に起動しており、その 10 秒の間にページを開いたことで発生した。

## 原因

複数の要因の組み合わせで「一度の起動時競合が恒久的な 0 件表示」になる。

1. `main.tsx` の QueryClient は `retry: 1` のため、connection refused の約 1 秒後の 1 回しかリトライしない。backend 起動前の失敗はリトライも失敗する
2. `refetchOnWindowFocus: false` のため、ウィンドウ再フォーカスでの再取得が無い
3. `['aws', 'profiles']` クエリの observer は App.tsx に常駐する `useProfiles()` の 1 つだけで、アンマウントされないため refetchOnMount の機会も無い
4. 通常操作で `['aws', 'profiles']` を invalidate する経路が無い (TopBar の Refresh は AWS ビュー表示中に押せば `['aws']` プレフィックスで一致するが、ユーザーが復旧手段として認知できない。AccountView の SSO 期限切れ検知による invalidate はアクティブプロファイルが存在しないと発動しない)
5. `App.tsx` はクエリの error を `console.error` に出力するだけで UI に表示しないため、エラー状態と「本当に 0 件」の区別がつかない

useProfiles のワンショット自動オープンは「error では shot を消費しない (refetch で改めて発火できる)」設計だが、上記により refetch 自体が発生しないため機能しない。

## 暫定回避策

- backend 起動後にブラウザをリロードする
- または AWS ビューを表示した状態で TopBar の Refresh ボタンを押す (`['aws']` の invalidate に profiles が含まれる)

## 修正案

1. `api/queries.ts` の `useProfiles` / `useGcpProjects` に、エラー状態の間だけポーリングする `refetchInterval` を追加する (例: `(query) => (query.state.status === 'error' ? 15_000 : false)`)。backend が起動し次第自動復旧し、成功後はポーリングしない
2. AddSessionPicker にエラー表示を追加し、一覧取得失敗時は「0件」ではなく取得エラーである旨と再試行導線を表示する
3. AWS ピッカーにも GCP と対称の再取得ボタンを設ける (または 2 の再試行導線で兼ねる)

## 解決方法

- `frontend/src/api/queries.ts` の `useProfiles` / `useGcpProjects` に `refetchInterval` を追加し、クエリが `error` 状態の間だけ 15 秒間隔でポーリングするようにした。成功後は `false` を返しポーリングを止める
- `frontend/src/components/session/AddSessionPicker.tsx` に `loadError` / `onRetry` プロパティを追加し、取得失敗時は「一覧の取得に失敗しました」+ 再試行ボタンを表示するようにした (この間は空状態メッセージを出さない)
- `frontend/src/hooks/useProfiles.ts` / `useGcpProjects.ts` の戻り値に `isError` / `refetchProfiles` を追加し、`AwsSessionTabs.tsx` / `GcpSessionTabs.tsx` からピッカーへ配線した
- AWS ピッカーのヘッダーにも GCP と対称の再取得ボタン (`Icons.refresh`) を追加した
