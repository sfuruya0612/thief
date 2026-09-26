# 0020 AWS / Google Cloud のマルチアカウント表示をセッションタブで切り替えられるようにする

Created: 2026-07-17
Completed: 2026-07-17
Model: Claude Fable 5 claude-fable-5

## 背景 / 根拠

thief は現在、AWS プロファイルと GCP プロジェクトを単一選択ドロップダウン (`ProfileSelect` / `GcpProjectSelect`) で切り替える設計であり、複数アカウントを同時に開いて行き来する手段がない。複数環境 (dev / stg / prod) や複数アカウントを横断して調査する運用では、切り替えのたびに選択操作が必要で、クエリエディタの作業文脈も見失いやすい。

claude.ai/design プロジェクト (3534c8e7-8416-413b-9dd6-ec12a293d303) の `Query Editor.dc.html` セクション 4/5/6/7a で「セッションタブ」方式のマルチアカウント UI が確定したため、これを実装する。

## 対応内容

- TopBar 直下にセッションタブバーを常設する (AWS ビュー = プロファイル、GCP ビュー = プロジェクト。Datadog / TiDB は対象外)
- タブの追加はタブバーの ＋ ボタン直下のピッカーポップオーバーで行う (検索付き、開設済みはグレーアウト)
- タブが収まらない場合はモック 7a の「他 N ▾」オーバーフローメニュー方式を AWS / GCP 共通で採用する
- サイドバーの選択ドロップダウンは廃止し、「アクティブセッション」カードに置換する
- backend の `/api/aws/profiles` を拡張し、認証方式 (sso / access_key / assume_role / credential_process / unknown) と SSO セッションの有効期限を返す。ピッカーの SSO 状態バッジとアクティブセッションカードの有効期限表示に使う
- 開いているタブとアクティブタブは localStorage (`cloudlens:v1`) に永続化し、既存の単一選択フィールド (`activeProfile` / `gcpProject`) からマイグレーションする

## スコープ外

- ピッカーからの `aws sso login` プロセス起動 (期限切れプロファイルは開いた後に既存 SSOExpiredBanner が案内する)
- GCP の ADC identity / 有効期限の表示 (backend に API がないため。後続 issue 候補)
- region のタブ別保持 (グローバル単一のまま)

## 解決方法

### backend

- `internal/aws/profiles.go` を全面書き換え、`AuthType` / `SSOStatus` の string enum と `Profile` 構造体 (Region / AuthType / SSOStatus / SSOExpiresAt を追加) を導入した
- `~/.aws/config` のパースを `parseAWSConfig` に置き換え、`[profile]` / `[default]` に加えて `[sso-session]` セクションを収集するようにした。`~/.aws/credentials` も読み、credentials のみに定義されたプロファイルも一覧に含める (`ValidateProfileName` 非準拠の名前は除外 + Warn)
- 認証方式は `resolveAuthType` で `role_arn` → `assume_role`、`sso_session` / `sso_start_url` → `sso`、`aws_access_key_id` → `access_key`、`credential_process` → `credential_process`、その他 → `unknown` の優先順で判定する
- `internal/aws/sso_cache.go` を新規作成し、`~/.aws/sso/cache` の全 JSON を 1 パス走査して startUrl (trailing slash 正規化) で突き合わせ、`valid` / `expired` / `not_logged_in` を判定する。パース struct は startUrl / expiresAt のみで accessToken はメモリに展開しない。ファイル名の SHA1 推測は形式差 (session 名 vs start_url) で不一致を起こすため採用しない
- 一覧 API はローカルファイルの best-effort 静的ビューとして 500 にせず、読み取り失敗は slog.Warn + degrade (SSOStatus 欠落 = バッジなし) とした
- `ProfileInfo` DTO に `region` / `auth_type` (必須) / `sso_status` / `sso_expires_at` (RFC3339 UTC、omitempty) を追加した

### frontend

- `lib/sessionTabsState.ts` (open / activate / close / move / swapToVisible / normalize の純関数)、`lib/sessionMeta.ts` (環境色・バッジ・期限表示・ピッカー項目)、`lib/sessionTabsLayout.ts` (7a オーバーフローの表示本数計算) を新規作成した
- `lib/storage.ts` の `PersistedState` に `awsSessions` / `gcpSessions` を追加し、旧 `activeProfile` / `gcpProject` からのマイグレーションと旧⇄新往復の reconcile (旧側で選択変更 → openSession で合流) を実装した。新版はアクティブタブを旧フィールドへ常にミラーし、ロールバック互換を保つ
- `hooks/useSessionTabs.ts` を新規作成し、`useProfiles` / `useGcpProjects` をセッションタブ API (openProfiles / activateProfile / openProfile / closeProfile / moveProfile / swapProfileToVisible 等) に書き換えた。初回のワンショット自動オープンは一覧取得成功時のみ消費し、全タブ閉後は自動再オープンしない
- `components/session/` に SessionTabs (タブバー本体、ResizeObserver 幅計測 + useMemo 派生の表示本数、他 N ▾ メニュー、Ctrl+1-9、DnD 並べ替え)、AddSessionPicker (検索 + キーボード操作 + 開設済みグレーアウト)、AwsSessionTabs / GcpSessionTabs (組立)、AwsActiveSessionCard / GcpActiveSessionCard (サイドバー置換カード)、SessionEmptyState を新規作成した
- `App.tsx` にタブバーを配置し、`key={activeProfile}` / `key={gcpProject}` の再マウントでタブ切替時のフィルタ・選択・Drawer の残留を防いだ。SSO 期限切れ検知時は profiles クエリを invalidate してバッジを追随させる
- `ProfileSelect` / `GcpProjectSelect` を削除し、`app.css` の `.profile-select*` を `.session-*` 一式に置き換えた

### 検証

- `mise run check` (fmt + lint + backend / frontend テスト) 全通過。frontend 270 テスト、backend は race + cover 付きで通過
- 実環境 (プロファイル 152 件) で `/api/aws/profiles` を確認し、sso 149 件 (valid 143 / expired 6)、credentials-only の access_key 3 件が一覧に含まれ、非 sso プロファイルに sso_status / sso_expires_at が付かない契約を確認した
- `mise run frontend:build` (tsc -b + vite build) 通過
