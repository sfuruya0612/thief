# AGENTS

原則・レビュー・コミット・issues・変更履歴などの汎用規約はグローバル `~/.codex/AGENTS.md` に集約されている。本ファイルはこのリポジトリ固有の規約のみを記載する。

## pre-commit

- mise run fmt / mise run check / mise run test を実行すること

## リポジトリ概要

このリポジトリは backend (Go) と frontend (React) を含むモノレポ。

- backend/  : Go で実装された API サーバ・CLI
- frontend/ : Vite + React + TypeScript で実装された Web クライアント

## タスクランナー (mise run)

リポジトリ共通のタスクは **`mise run`** で統一する。ビルド・テスト・Lint 等を実行する際、可能な限り個別のコマンドを直接叩かず `mise run <task>` を経由すること。Makefile は存在しない。

### ルートタスク (集約)

| タスク | 内容 |
| --- | --- |
| `mise run setup` | 初回セットアップ (依存取得、ツール導入) |
| `mise run fmt` | backend / frontend 両方のフォーマッタを実行 |
| `mise run lint` | 両方の Lint を実行 |
| `mise run test` | 両方のテストを実行 (ユニット中心) |
| `mise run check` | `fmt` + `lint` + `test` を順に実行 (PR 提出前の最終確認) |

### backend タスク

| タスク | 内容 |
| --- | --- |
| `mise run backend:build` | `go build ./...` |
| `mise run backend:install` | `thief` CLI を `go install ./cmd/thief` で `$GOPATH/bin` に導入 |
| `mise run backend:test` | `go test -race -cover ./...` |
| `mise run backend:lint` | `go vet` + `staticcheck` + `govulncheck` + `golangci-lint` |
| `mise run backend:fmt` | `gofmt -w .` + `goimports -w .` |
| `mise run backend:tidy` | `go mod tidy -v` |
| `mise run backend:mocks` | mockery でモック生成 |
| `mise run backend:run` | ローカルで API サーバを起動 (127.0.0.1:8080) |

### frontend タスク

| タスク | 内容 |
| --- | --- |
| `mise run frontend:setup` | `npm install` |
| `mise run frontend:build` | `npm run build` (`tsc -b && vite build`) |
| `mise run frontend:test` | `npm run test` (vitest) |
| `mise run frontend:lint` | `npm run lint` (eslint + `tsc --noEmit`) |
| `mise run frontend:fmt` | `npm run fmt` (prettier) |
| `mise run frontend:run` | `npm run dev` (vite dev server, http://localhost:8082) |
| `mise run frontend:serve` | `npm run build` + `npm run preview` (ポート 8082 で dist を配信) |

### Agent への指示

- 「テストを実行して」「ビルドが通るか確認して」と指示された場合、まず `mise.toml` を確認し、対応するタスクがあれば `mise run <task>` で実行する。
- `mise.toml` に存在しないタスクを繰り返し直接コマンドで実行している場合、`mise.toml` への追加を提案すること。
- frontend タスクは `dir = "frontend"`、backend タスクは `dir = "backend"` で実行されるため、カレントディレクトリを変更しなくてもルートから実行できる。

## backend (Go)

### 基本方針

- **対象言語/バージョン**: Go 1.26 系を前提とする(現行最新は 1.26.x)。`go.mod` の `go` ディレクティブは `go 1.26` を基本とし、リポジトリ既存値が古い場合のみそれに揃える。
- **設計原則**: **堅牢性 > 性能**。性能最適化はプロファイリングで裏付けがある場合にのみ行い、可読性・保守性・正確性を犠牲にしない。
- **対象アプリケーション**: CLI ツール および Web API サーバの両方。
- **シンプルさ**: 標準ライブラリで十分なものは標準ライブラリを使う。抽象化は実需が出てから入れる(YAGNI)。

### 依存関係の方針

依存は最小限に保つ。追加可否は以下の優先順で判断する。

1. **Go 標準ライブラリ** — 第一選択。HTTP サーバなら `net/http`、JSON は `encoding/json`、ロギングは `log/slog` を用いる。
2. **`golang.org/x/...` 系および準標準扱いの軽量ライブラリ** — 標準で不足する場合に許容。
3. **CLI 向け定番ライブラリ** — `spf13/cobra`(コマンド構造)、必要に応じて `spf13/viper`(設定)を許容。
4. **クラウドプロバイダ公式 SDK** — AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2/...`)、GCP・OCI 公式 SDK 等は許容。
5. **その他のサードパーティ依存** — 上記で代替できない場合のみ。追加する場合は PR 説明で「なぜ標準ライブラリでは不足か」「メンテナンス状態」「ライセンス」を明示すること。

ORM、リッチなロガー(zap/zerolog 等)、自前 DI コンテナなどは原則導入しない。標準 `database/sql`(必要なら `sqlc` で生成)、`log/slog`、関数引数によるコンストラクタ DI を用いる。

### プロジェクト構造

CLI と Web API サーバを単一リポジトリで扱う場合の標準構造:

```
backend/
├── cmd/
│   ├── <app-name>/          # 各エントリポイント (main パッケージ)
│   │   └── main.go
│   └── <other-cli>/
│       └── main.go
├── internal/                # 外部からの import を禁止する内部パッケージ
│   ├── <domain>/            # ドメインごとに分割
│   ├── handler/ または server/  # Web API のハンドラ層
│   ├── command/             # CLI のサブコマンド実装 (cobra Command を返す関数群)
│   ├── service/             # ユースケース層
│   ├── repository/          # 永続化層 (interface とその実装)
│   ├── config/              # 設定の読み込み・検証
│   └── logging/             # slog の初期化
├── pkg/                     # 外部公開する再利用可能なパッケージ (慎重に)
├── go.mod
└── go.sum
```

- 公開する必要がないものはすべて `internal/` 配下に置く。`pkg/` は意図的に外部公開する場合のみ。
- 1 パッケージ 1 責務。パッケージ名は短く小文字(`util`, `common`, `helpers` のような曖昧名は避ける)。
- 循環参照は禁止。レイヤ間は上位 → 下位の単方向依存とする(`handler` → `service` → `repository`)。

### エラーハンドリング

- **必ず最終的に処理されるか、上位へ伝播されること**。`_` で握り潰すのは原則禁止(明確な理由をコメントで残せる場合のみ可)。
- **ラップは `fmt.Errorf` の `%w` 動詞**を使う。

  ```go
  if err := repo.Save(ctx, u); err != nil {
      return fmt.Errorf("save user %s: %w", u.ID, err)
  }
  ```

- 比較は `errors.Is`、型取り出しは `errors.As` を用いる。`==` での直接比較は避ける(センチネルエラーであっても)。
- センチネルエラーは公開する場合 `Err` プレフィックスを付ける(例: `ErrNotFound`)。
- 自前のエラー型はフィールドにコンテキスト情報を持たせ、`Error() string` と `Unwrap() error` を実装する。
- HTTP ハンドラでは、ドメインエラーをステータスコードへマップする層を 1 箇所に集約する(handler 層に `errorToHTTP` 等を置く)。
- **`panic` の使用は初期化失敗(プロセス起動を中止すべき場合)に限定する**。リクエスト処理中の `panic` は禁止。`recover` は `net/http` のミドルウェア最上位など、限定された場所でのみ使う。

### context.Context

- I/O や時間のかかる処理を行うすべての関数は **第一引数で `ctx context.Context` を受け取る**。
- `ctx` を構造体フィールドに保持しない。関数引数で受け渡す。
- 既存の `ctx` がない最上位(`main` 関数や CLI コマンド)では `context.Background()` を起点に、`signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` を用いてシグナル連動キャンセルを設定する。
- HTTP サーバは `Server.Shutdown(ctx)` を用いてグレースフルシャットダウンを実装する(タイムアウトは設定可能に)。
- `context.WithTimeout` / `WithCancel` で派生した場合、**必ず `defer cancel()` を呼ぶ**。
- `context.Value` での値受け渡しは、リクエストスコープのメタデータ(リクエスト ID、認証主体等)に限定する。アプリケーション依存の値は引数で渡す。

### 並行処理

- goroutine を起動するときは **必ず終了経路を確保する**。ループ内で起動して放置するのは禁止。
- 複数 goroutine の協調には `golang.org/x/sync/errgroup` を推奨(エラー伝播とキャンセルが揃う)。
- `sync.WaitGroup` を使う場合は `Add` → `go func() { defer wg.Done(); ... }()` のパターンを徹底する。
- チャネルは送信側がクローズする。受信側はクローズしない。
- 共有データの保護は `sync.Mutex` / `sync.RWMutex` を使い、ロックの取得順序を一貫させる。複雑なロック構造より、データオーナーシップを 1 goroutine に集約してチャネルでやり取りする方を優先する。
- データ競合検出のため、テストおよび CI では `go test -race ./...` を必ず実行する。

### ロギング (`log/slog`)

- ログは構造化ログ(`log/slog`)に統一。`fmt.Println`、`log.Printf` での恒久ログは禁止(デバッグ中の一時利用は可、PR 提出前に削除)。
- アプリケーション起動時にハンドラ(`slog.NewJSONHandler` 推奨)を構成し、`slog.SetDefault` するか、コンストラクタで明示的に注入する。
- ログレベル: `Debug` / `Info` / `Warn` / `Error` を使い分ける。本番デフォルトは `Info`。
- リクエストごとに `slog.Logger` を派生させ、`request_id`、`user_id` 等を `With` で付与してから `ctx` に載せて伝播させる(または引数で渡す)。
- 機密情報(パスワード、トークン、PII)はログに含めない。構造体に `String()` メソッドを実装してマスクするか、ログ用 DTO に変換する。
- エラーログは `slog.Error("...", "err", err)` のように `err` 属性を付ける。スタックトレースが必要なら `runtime.Stack` を別途付与する。

### 設定

- 設定の優先順位: コマンドライン引数 > 環境変数 > 設定ファイル > デフォルト値。
- 環境変数は `<APP>_FOO_BAR` のように接頭辞を付ける。
- 起動時に**設定の検証(必須項目、値域)を行い、不正なら `panic` ではなく `os.Exit(1)` 相当で終了**(エラーメッセージを `slog.Error` で出してから)。
- シークレットはコードベースに含めない。環境変数または Secrets Manager 経由で注入する。

### HTTP / Web API サーバ

- サーバは `net/http` の `http.Server` を直接使う。`http.ListenAndServe` のラッパーではなく、`Server` 構造体に `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout` を必ず設定する(`ReadHeaderTimeout` は最低限必須)。
- ルーティングは Go 1.22+ の `http.ServeMux`(メソッド別パターン対応)で十分。複雑な要件があるときのみ軽量ルータの追加を検討する。
- ミドルウェアチェーンは関数合成で組む。標準的に入れるもの: パニックリカバリ、リクエスト ID 付与、構造化アクセスログ、タイムアウト、リクエストサイズ制限。
- ハンドラは `func(w http.ResponseWriter, r *http.Request)` の形を保ちつつ、内部でユースケース層(service)を呼ぶ。ハンドラ内にビジネスロジックを書かない。
- レスポンスの JSON エンコードは `json.NewEncoder(w).Encode(v)` を使う。エラー時は統一フォーマット(`{"error": {"code": "...", "message": "..."}}` 等)で返す。
- グレースフルシャットダウン: `signal.NotifyContext` で受けたキャンセルで `srv.Shutdown(ctx)` を呼ぶ。

### CLI (Cobra)

- コマンドツリーは `internal/command/` に各コマンドを定義する関数として配置(`func NewRootCmd() *cobra.Command` など)。
- `RunE` を使い、エラーは return する(Cobra が自動表示)。
- フラグはローカルフラグを優先。グローバル設定はルートコマンドの `PersistentPreRunE` で初期化する。
- 出力は `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` を使い、テストで差し替え可能にする。

### backend テスト

- テストフレームワークは標準 `testing` を使う。アサーションライブラリは原則不要(`reflect.DeepEqual` や `errors.Is` で十分)。必要なら `github.com/google/go-cmp/cmp` を許容。
- **テーブル駆動テスト**を基本とする:

  ```go
  func TestParse(t *testing.T) {
      tests := []struct {
          name    string
          input   string
          want    Result
          wantErr error
      }{
          {name: "ok", input: "1", want: Result{N: 1}},
          {name: "empty", input: "", wantErr: ErrEmpty},
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              got, err := Parse(tt.input)
              if !errors.Is(err, tt.wantErr) {
                  t.Fatalf("err = %v, want %v", err, tt.wantErr)
              }
              if diff := cmp.Diff(tt.want, got); diff != "" {
                  t.Errorf("mismatch (-want +got):\n%s", diff)
              }
          })
      }
  }
  ```

- ヘルパー関数では **`t.Helper()` を必ず呼ぶ**(失敗時の行番号が正しく出る)。
- リソース解放は `t.Cleanup(func(){ ... })` を使う(`defer` より先に登録順序を意識せずに済む)。
- **モックの活用**: 依存はインターフェース経由で受け取り、テストではモックを差し込む。モック生成には `go.uber.org/mock`(旧 gomock の後継)または手書きモックを使う。`testify/mock` のような追加 DSL は原則使わない。
- HTTP ハンドラのテストは `net/http/httptest` を使う。
- 並行テストは `t.Parallel()` を活用するが、共有状態に注意する。
- カバレッジ目標値は設けないが、ロジック分岐とエラーパスは必ずテストする。

### backend コーディングスタイル

- フォーマットは `gofmt` / `goimports`。
- 静的解析は `go vet ./...` と `staticcheck ./...` を CI で実行する。
- 受け取り側はインターフェース、返却側は具象型(consumer-defined interfaces)。
- 構造体のゼロ値が有効に使えるなら、コンストラクタを強要しない。`New` 系コンストラクタは「不変条件を満たす初期化」が必要なときに用意する。
- exported な識別子には godoc コメントを付け、`// Foo は ...` の形式で名前から書き出す。
- 早期 return を活用し、ネストを浅く保つ。
- マジックナンバーは定数化する。

### backend ビルドと CI

- ローカルでの最低限のチェック: `go build ./...` / `go vet ./...` / `go test -race ./...` / `gofmt -l .`。
- バイナリビルドは `CGO_ENABLED=0` を基本とする(static link で配布が容易)。
- バージョン情報は `-ldflags` で `main.version` に注入する。

### backend Coding Agent への指示

- 本節の方針と矛盾するコードを見つけた場合、**勝手に大規模修正せず、まず差分の方針を提案して合意を得る**。
- 新規依存追加が必要なときは、追加前に「標準ライブラリで代替できないか」を必ず検討し、その結論を PR / 応答に明記する。
- パフォーマンス目的のリファクタは、ベンチマーク(`testing.B`)による測定根拠とセットで提案する。
- エラー処理を省略・簡略化したいケース(例: テストのセットアップ)では、`t.Fatal` でフェイルさせる方針を取り、本番コードのエラー処理規律をテストに持ち込まない。
- 不明点はコードを書き始める前に質問する。とくに以下は質問が望ましい:
  - 公開 API の互換性に影響する変更
  - 永続化スキーマや外部連携の変更
  - 新規依存ライブラリの追加
  - 並行処理の同期戦略の変更

## frontend (React)

### 基本方針

- **対象スタック**: Vite + React 18 + TypeScript (strict)。ビルド対象は **Web ブラウザのみ**(Flutter 時代の macOS Desktop 対応は廃止済み)。
- **状態管理**: サーバ状態は **TanStack Query (`@tanstack/react-query`)**、UI 状態は `useState`/`useReducer` + カスタムフック。Redux/Zustand/Riverpod 相当のライブラリは導入しない(YAGNI)。
- **ルーティング**: react-router 等は導入しない。profile タブ・サービス選択・トップレベルビュー (`AppView`: `aws`/`bigquery`/`datadog`/`tidb`) は React state + `localStorage`(`cloudlens:v1` キー、`lib/storage.ts`)で管理する。
- **多言語対応**: 現状は日本語 UI 文字列のハードコードのみ。i18n ライブラリは未導入(需要が出るまで導入しない)。

### ディレクトリ構造

```
frontend/
├── index.html
├── package.json / vite.config.ts / tsconfig.json
├── eslint.config.js / .prettierrc.json
├── public/assets/
└── src/
    ├── main.tsx / App.tsx / app.css     # エントリポイントとレイアウト CSS
    ├── types/{aws,nonaws,common}.ts     # Raw (backend JSON 形状) と Row (UI 形状) の 2 層
    ├── api/{client,endpoints,queries}.ts
    ├── lib/{normalize,normalizeNonAws,serviceMeta,format,storage,sessionTabsState,sessionMeta,sessionTabsLayout}.ts
    ├── hooks/{useTweaks,useProfiles,useGcpProjects,useSessionTabs}.ts
    ├── components/
    │   ├── {TopBar,Sidebar,StatsRow,FacetBar,StatusBar,DataTable,TweaksPanel,SSOExpiredBanner}.tsx
    │   ├── session/{SessionTabs,AddSessionPicker,AwsSessionTabs,GcpSessionTabs,AwsActiveSessionCard,GcpActiveSessionCard,SessionEmptyState}.tsx
    │   ├── Drawer/{Drawer,DrawerTags,DrawerLogs,DrawerEmpty,overviewRows}.tsx
    │   ├── icons/{Icons,AwsIcons,Spark,AreaChart}.tsx
    │   ├── primitives/{StatusBadge,TagList,CellBar,Money,Kbd}.tsx
    │   └── tables/{columns,nonAwsColumns}.tsx
    └── views/
        ├── AccountView.tsx              # AWS 15 サービス共通の ServicePanel
        └── nonaws/{BigQueryView,DatadogView,TiDBView}.tsx
```

- `types/*.ts` は Raw(バックエンド snake_case JSON)と Row(UI camelCase 表示用)を明確に分離する。変換は `lib/normalize.ts` / `lib/normalizeNonAws.ts` の純関数(`xxxFromRaw`)に集約する。
- feature-first ではなく、種類別(`api`/`lib`/`hooks`/`components`/`views`)にディレクトリを切る。React + TanStack Query 規模のアプリではこの方が把握しやすい。

### API クライアントとエラーハンドリング

- `api/client.ts` の `apiGet<T>`/`apiPost<T>` を経由してすべての HTTP 呼び出しを行う。`baseURL` は `import.meta.env.VITE_API_BASE ?? 'http://127.0.0.1:8080'`。
- 非 2xx レスポンスは `{"error", "code", "message", "details"}` 形状を parse し `ApiError`(`types/common.ts`)を throw する。ネットワーク到達不能等は `ApiError(0, 'network_error', ...)` に正規化する。
- SSO トークン期限切れは HTTP 401 + `code === 'SSO_TOKEN_EXPIRED'` で表現される。バックエンドの全 AWS リソースハンドラがこの経路を通るため、フロントは専用ステータス取得を持たず、各 `useResources` 呼び出しの `error` を `error instanceof ApiError && error.code === 'SSO_TOKEN_EXPIRED'` で判定し、`SSOExpiredBanner` を表示する。

### サーバ状態 (TanStack Query)

- `queryKey` はドメインを先頭に置く配列(`['aws', service, profile, region]`、`['bigquery', 'tables', dataset, projectId]` 等)で構成する。
- `staleTime` はバックエンドのキャッシュ TTL に合わせて `60_000`(60秒)を基本とする。プロファイル一覧のように変化が少ないものは `5 * 60 * 1000` 等に緩める。
- 依存データの遅延取得は `enabled: !!dependency` で制御する(例: `useBQTables` は `dataset` が確定するまで無効)。
- 更新系は `useMutation` + `queryClient.invalidateQueries({ queryKey: [...] })` で書く。トップバーの Refresh ボタンは現在表示中の `AppView` に対応する `queryKey` のみを invalidate する。

### UI 状態と永続化

- テーマ・密度・アクセントカラー等は `hooks/useTweaks.ts` の `Tweaks` 型に集約し、`document.documentElement` の `data-theme`/`data-density`/`data-accent` 属性に反映する(CSS 側は属性セレクタで分岐)。
- `region`/`activeProfile`/`view`/`tweaks` はそれぞれ `usePersistedXxx()` 形式の薄いフックで `lib/storage.ts` の `loadPersisted()`/`savePersisted()` を介し `localStorage` に永続化する。新しい永続化フィールドを追加する場合はこのパターンに従い、`PersistedState` インターフェースにフィールドを追加する。

### コンポーネント設計

- サービスごとに異なる Raw/Row 型を扱う AWS リソース表示は、`AccountView.tsx` の汎用 `ServicePanel<TRaw, TRow>` に `normalizer`/`columns`/`overviewRows` を渡す形で分岐する。サービス追加時もこのパターンに従う。
- テーブル列定義は `components/tables/columns.tsx`(`ColumnDef<T>` 型、`Dash()` ヘルパー、`formatBytes()` 等の共通ヘルパー)に集約する。非 AWS ドメイン(BigQuery/Datadog/TiDB)の列定義は `nonAwsColumns.tsx` に分離し、既存の AWS 専用 `columns.tsx` を汚さない。
- 汎用アイコンはインライン SVG(`components/icons/Icons.tsx`)で持つ。外部アイコンパッケージは導入しない。
- AWS サービスアイコン(`components/icons/AwsIcons.tsx`)は `public/assets/aws-icons/<service>.svg` を `<img>` で参照する。この SVG は AWS 公式 Architecture Icons の無改変ファイルであり、ライセンス上リポジトリにコミットしない(`.gitignore` 対象)。初回セットアップ時に `mise run frontend:fetch-aws-icons <zip>` で AWS 公式アイコンパッケージ(zip)から展開する。対応関係は `frontend/scripts/fetch-aws-icons.mjs` の `ICON_FILENAMES` を参照(natgw のみ Architecture Icons に該当がなく Resource Icons の 48px 版を使用)。
- Google Cloud サービスアイコン(`components/icons/GcpIcons.tsx`)は `public/assets/gcp-icons/<service>.svg` を `<img>` で参照する。この SVG は Google Cloud 公式 Icons (Unique Icons) の無改変ファイルであり、ライセンス上リポジトリにコミットしない(`.gitignore` 対象)。初回セットアップ時に `mise run frontend:fetch-gcp-icons <zip>` で Google Cloud 公式アイコンパッケージ(zip)から展開する。対応関係は `frontend/scripts/fetch-gcp-icons.mjs` の `ICON_FILENAMES` を参照。

### コーディングスタイル

- TypeScript は `strict`、`noUnusedLocals`/`noUnusedParameters` を有効化(`tsconfig.json`)。意図的に未使用の引数は `_` プレフィックスを付ける(ESLint 側で `argsIgnorePattern: '^_'` を許容)。
- コンポーネントは関数コンポーネント + Hooks のみ。クラスコンポーネントは書かない。
- Props の型は `interface XxxProps` として明示し、コンポーネント本体でデストラクチャする。
- CSS は `app.css` に集約されたクラス(`.toolbar`/`.table-wrap`/`table.dt`/`.seg`/`.nav-item`/`.btn`/`.stats`/`.facets` 等)を再利用する。新しいクラスを追加する場合は既存の命名規則(BEM 風ではなく機能名そのまま)に合わせる。
- 状態を更新するだけの `useEffect`(`setState` を effect 内で直接呼ぶパターン)は既存コードに複数存在するが、新規追加時は可能な限りイベントハンドラや `useMemo` で代替できないか検討する。

### frontend テスト

- テストランナーは **Vitest**(`environment: 'jsdom'`、`src/setupTests.ts` で `@testing-library/jest-dom` を読み込む)。
- コンポーネントテストは `@testing-library/react` を使う。
- テストファイルは対象ファイルと同じディレクトリに `<対象>.test.ts(x)` として置く(例: `lib/format.ts` → `lib/format.test.ts`)。
- PBT やユニットテストの厳密な役割分担(グローバル `~/.codex/AGENTS.md` の「テストについて」節)は Rust プロジェクト向けの規約であり、frontend の TypeScript コードには適用しない。frontend では純関数(`normalize.ts`/`format.ts` 等)のユニットテストと、複雑な分岐を持つコンポーネントのテストを中心に書く。

### Lint / Format

- Lint: `eslint.config.js`(flat config)。`typescript-eslint` の `recommended` + `react-hooks`(`rules-of-hooks`/`exhaustive-deps` のみ有効化。v7 系の React Compiler 前提ルールである `set-state-in-effect` 等は本プロジェクトでは無効化している)+ `react-refresh`。
- Format: Prettier(`.prettierrc.json`)。`*.md` は対象外(`.prettierignore`)、CLAUDE.md 等のドキュメントの整形はスコープ外とする。
- `npm run lint` は `eslint . && tsc --noEmit` の順で実行する。型エラーと Lint エラーは両方ゼロを維持する(警告は許容するが、新規コードで警告を増やさないよう努める)。

### frontend Coding Agent への指示

- 本節の方針と矛盾するコードを見つけた場合、**勝手に大規模修正せず、まず差分の方針を提案して合意を得る**。
- 新規パッケージ追加時は、追加理由(標準/既存パッケージで代替できない理由)を明示する。
- 状態管理ライブラリ(Redux/Zustand/Jotai 等)を導入したくなる場面が出ても、まず TanStack Query + useState/useReducer での実現方法を検討する。
- API レスポンスの型は必ず Raw 型を経由し、UI コンポーネントに snake_case のフィールドを直接渡さない。
- 不明点はコードを書き始める前に質問する。とくに以下は質問が望ましい:
  - 新しいトップレベルビュー・ルーティング相当の追加
  - 永続化する状態フィールドの追加
  - 認証フロー(SSO)の挙動変更
  - 新規依存ライブラリの追加

