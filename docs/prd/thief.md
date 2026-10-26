# thief PRD

Created: 2026-09-26
Model: Claude Fable 5.1
Status: Draft
Updated: 2026-10-26

## 概要

thief は、AWS、Google Cloud、Datadog、TiDB Cloud のリソースとコストを、手元の端末から Web UI、CLI、全画面の TUI で閲覧する開発者向けツールである。
TUI は Web UI に相当する操作を端末の中で行うことを目指し、第 1 段階では AWS のリソースの一覧、Drawer (Overview と Tags)、EC2 と ECS への対話セッションを扱う。
1 つの Go バイナリ (`thief`) が API サーバ、TUI (`thief tui`)、CLI を兼ね、ブラウザで動く React の SPA がその API を使い、TUI は API サーバを経由せず CLI と同じ取得層をプロセス内で呼ぶ。

## 背景と課題

### 本 PRD を書く理由

thief の要求はこれまで `docs/issues/TODO.md` の箇条書きと個々の issue にしか書かれていない。
そのため、製品全体として何を満たすべきかを 1 か所で示す文書が無い。
本 PRD は、実装済みのコード、`docs/issues/` の issue 199 件 (closed 198 件と pending 1 件)、`docs/issues/TODO.md` から要求を逆算して書き起こしたものである。
  本 PRD の作成中に起票した issue 0200 (CORS の不具合) と issue 0217 (セッションのルートの Origin の検査) は材料に含めず、修正の結果だけを反映した。
以下では、これら 3 つを**材料**と呼ぶ。
TUI の記述 (概要と目的の TUI の文、成功指標の TUI の行、UC-14、FR-10 の `thief tui` の行、FR-29、FR-30、スコープの TUI の項目、非機能要求の性能とセキュリティの TUI の行、制約、未確定論点の TUI の行、決定した論点の 2026-10-23 の表、関連資料の ADR 0033) は 2026-10-23 の改訂で加えたものであり、改訂前の材料の issue と TODO には無い。
`docs/issues/TODO.md` の TUI の後続作業の 3 行は、この改訂で PRD と同時に加えたものであり、改訂前の材料ではない。
根拠は、同じ日に開発者が決めた TUI の追加 (「決定した論点」)、開発者が動作を確かめたプロトタイプ (`backend/internal/tui/`、`backend/internal/cli/tui.go`)、TUI が合わせる対象である Web UI の実装済みのコード (`frontend/src/lib/serviceMeta.ts`、`frontend/src/components/tables/columns.tsx`、`frontend/src/components/Drawer/overviewRows.tsx`) である。

- issue は 2026-07-12 から 2026-09-26 まで git 管理の対象外だった (コミット b4ca037、ADR 0027)。
  2026-09-26 に `docs/issues/` へ移し、git で管理することにした (ADR 0028)。
  その間に書いた issue の書き換えの経過は、リポジトリの履歴に残っていない。
- 設計判断の根拠は issue の本文とコミットメッセージに散っている。
  本 PRD は要求だけを扱い、判断の記録は `docs/adr/` に分けて置く。
- リポジトリ規約 `AGENTS.md` の記述と実装が食い違う箇所がある (「制約と前提」の「規約と実装の差」を参照)。
  規約だけを読んで実装の振る舞いを知ることはできない。

### thief が解く課題

材料に課題として書かれているもの、材料には課題として書かれておらず実装から推定したもの、2026-10-23 の TUI の追加の決定で挙げたものの 3 つを分けて示す。

材料に書かれている課題は、次のとおりである。

- 複数のアカウントを同時に開けない。
  issue 0020 は、単一選択のドロップダウンでは複数の AWS アカウントを同時に開けないことを課題としている。
  thief は AWS のプロファイルと Google Cloud のプロジェクトをタブで切り替える UI を持つ (`frontend/src/components/session/`、issue 0020)。
  Datadog の組織のタブは issue 0168 で加えた (`frontend/src/components/session/DatadogOrgSessionTabs.tsx`)。
- AWS の SSO トークンが切れると操作が失敗し、原因が画面から分からない。
  `docs/issues/TODO.md` は、SSO ログインの後も期限切れの表示が残る問題と、Cost Explorer の画面で期限切れが原因不明の 500 エラーに見える問題 (issue 0105) を挙げている。
  thief は期限切れを HTTP 401 `SSO_TOKEN_EXPIRED` として返し、画面上からデバイス認可 (RFC 8628) で再ログインさせる (`backend/internal/api/errors.go` `writeUnauthorized`、`backend/internal/ssoauth/ssoauth.go`、issue 0147〜0149)。
- ログの調査だけ Google Cloud のコンソールへ移る必要があり、アプリの中で調査が完結しない (issue 0022)。
  thief は Cloud Logging のログを検索し、Live Tail で表示する (`frontend/src/views/nonaws/CloudLoggingView.tsx`)。
- EC2 と ECS のシェルに接続したまま別の画面を見ると、接続をやり直す必要がある (`docs/issues/TODO.md`)。
  thief はブラウザのターミナルを画面の下部のドックに置き、画面を移っても接続を保つ (issue 0174)。

次は材料に課題として書かれていない。
実装の範囲から推定した課題であり、仮説である。

- EC2 と ECS のシェルに入るには、Session Manager プラグインを別途用意する必要がある。
  thief の Web の画面は、Go で SSM のデータチャネルを中継してブラウザのターミナルから接続させる (`backend/internal/session/bridge.go` `Bridge.Run`)。
  CLI の `thief ec2 session` と `thief ecs exec` は Session Manager プラグインを使う。
- コストの確認先が AWS Cost Explorer、Datadog、TiDB Cloud に分かれている。
  thief はそれぞれのコストを同じ UI で表示する (`frontend/src/views/CostExplorerPanel.tsx`、`frontend/src/views/nonaws/DatadogView.tsx`、`frontend/src/views/nonaws/TiDBView.tsx`)。
- Athena、CloudWatch Logs、BigQuery の調査で、それぞれのコンソールを行き来する必要がある。
  thief はこれらを 1 つの UI で検索させる (`frontend/src/views/AthenaView.tsx`、`frontend/src/views/CloudWatchLogsView.tsx`、`frontend/src/views/nonaws/BigQueryView.tsx`)。

次は 2026-10-23 に開発者が TUI の追加を決めたときの課題である (ADR 0033 の「状況」)。

- 端末の中だけで、プロファイル、リージョン、サービスを切り替えながら一覧と Drawer (Overview と Tags) を見比べる手段が無い。
  Web UI を使うには `thief server` で API サーバを起動し、ブラウザで frontend を開く必要がある。
  CLI の一覧コマンド (`thief ec2 ls` など) は 1 回の出力で終わる。
  thief は全画面の TUI (`thief tui`) で、API サーバとブラウザを起動せずに一覧、Drawer、対話セッションを行わせる (FR-29、FR-30)。

## 目的

- 利用者が Web UI から、4 つの外部サービス (AWS、Google Cloud、Datadog、TiDB Cloud) のリソース一覧、詳細、コストに到達できる。
- 利用者がブラウザだけで EC2 と ECS のシェルに入れる。
- 利用者がブラウザと API サーバを起動せずに、端末の中だけで AWS のリソースの一覧と Drawer (Overview と Tags) を確認し、EC2 と ECS のシェルに入れる (TUI の第 1 段階)。
  TUI の最終的な目標は Web UI に相当する操作であり、第 2 段階以降の範囲と順序は「未確定論点」で決める。
- 利用者が SSO トークンの期限切れを画面上で知り、画面を離れずに再ログインできる。
- 利用者が CLI からも Web UI と同じ主要なリソースを一覧でき、結果をタブ区切りか CSV で受け取れる。
- 利用者の認証情報と、外部サービスから取得したデータを、利用者の端末の外に送らない。
  これらの送り先は各外部サービスの公式 API だけである。

## 成功指標

材料には利用状況の計測の記録が無い。
そのため、本 PRD の成功指標はリポジトリ上で機械的に測れるものに限る。
利用状況の指標 (利用頻度、コンソールを開く回数の削減など) は置かない (「決定した論点」)。

| 指標 | 目標 | 測定方法 |
| --- | --- | --- |
| 品質ゲートの通過 | fmt、lint、vuln、test がすべて成功する | リポジトリのルートで `mise run check` を実行し、終了コードが 0 であることを確認する |
| backend と frontend の型契約の一致 | 不一致 0 件 | `frontend/` で `npm run lint` を実行する。`frontend/src/types/contract.check.ts` の型検査が失敗しないことを確認する |
| AWS サービスの網羅 | `frontend/src/lib/serviceMeta.ts` の `SERVICES` にある 23 サービスすべてに、一覧の表示先がある | `SERVICES` の各 key が、`SERVICE_TO_PATH` の対応先か、専用ビュー (athena、cloudwatchlogs、costexplorer、pricing) のどちらかを持つことをコードで確認する |
| CLI の一覧コマンドの存在 | FR-10 の表の一覧コマンドが 27 個すべて存在する。Web にだけある機能 (Pricing、分割表示など) は対象外とする | 表の各コマンドに `--help` を付けて実行し、終了コードが 0 で、出力の `Usage:` の次の行が、前後の空白を除いたとき表のコマンドで始まることを確認する |
| TUI の AWS サービスの網羅 | TUI の第 1 段階の完了時に、FR-29 の対象サービス 19 個の ID すべてに TUI の一覧がある。2026-10-26 時点で 19 個すべてにある (issue 0219 から 0223) | `backend/internal/cli/tui.go` で `tui.Deps` を組み立てる処理を関数に切り出し、その `Services` の各要素の `ID()` の集合が FR-29 に挙げた 19 個の ID の集合と一致することをテストで確認する (`newTUIDeps` と `backend/internal/cli/tui_test.go`。2026-10-26 時点の期待値は 19 個すべてである)。各要素の `Load` が、Web UI の同じ一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が使う `backend/internal/aws/` の関数 (`ListEC2Resources`、`ListSSMParameters`、`ListCFNStacks`、`ListSecretResources` など。名前はサービスごとに異なり、CLI だけが使う `ListSSMParameterInfos` や `ListCfnStackSummaries` ではない) を呼ぶことと、列、Overview、Tags の一致は、FR-29 の受け入れ基準の一覧関数の項目と列の項目で確認する |

## ユーザーとユースケース

### 想定ユーザー

- 複数の AWS アカウントと Google Cloud プロジェクトを運用する開発者と SRE。
  自分の端末で thief を起動して使う。
- 利用者は自分の認証情報 (`~/.aws/`、Google Cloud の ADC、Datadog の OAuth トークンか API キー、TiDB Cloud の API キー) を持つ。

### ユースケース

| ID | 場面 | 利用者の行動 | 対応する機能要求 |
| --- | --- | --- | --- |
| UC-1 | 障害調査の初動 | AWS のプロファイルとリージョンを選び、EC2、ECS、RDS などの状態を一覧と詳細で確認し、ECS のタスク数の推移を見る。古い結果を捨てて取得し直す | FR-1、FR-2、FR-3、FR-4、FR-28 |
| UC-2 | コンテナやインスタンスへの接続 | ECS のタスクか EC2 インスタンスを選び、ブラウザのターミナルからシェルに入る。複数のセッションを開いたまま別の画面を見る | FR-7、FR-8 |
| UC-3 | SSO トークンの期限切れ | 一覧の取得で期限切れを知り、その場でデバイス認可を完了して一覧を再表示する | FR-9 |
| UC-4 | 設定値の確認と修正 | Secrets Manager か Parameter Store の値を開き、確認のうえ上書きする | FR-5 |
| UC-5 | オブジェクトの確認と差し替え | S3 か GCS のオブジェクトを一覧し、プレビュー、ダウンロード、アップロード、テキストの編集、SQL 検索を行う | FR-6、FR-17、FR-20 |
| UC-6 | コストの把握 | AWS、Datadog、TiDB Cloud のコストを確認する。AWS は集計表の CSV 出力と、CLI での予測も使う | FR-11、FR-21、FR-22、FR-23、FR-24 |
| UC-7 | 料金の比較 | AWS の料金をインスタンスタイプ、ファミリ、購入方式で絞り込んで比べ、行と数量を選んで月額の見積もりを出す | FR-12 |
| UC-8 | ログとデータの調査 | Athena、BigQuery で読み取りの SQL を実行し、CloudWatch Logs、Cloud Logging で期間とフィルタを指定してログを検索し、Live Tail で追う。よく使うクエリを保存して呼び出す | FR-13、FR-14、FR-15、FR-16、FR-18、FR-19 |
| UC-9 | Google Cloud のリソース確認 | プロジェクトを選び、Cloud Run、GCS、IAM、サービスアカウントを一覧する | FR-20 |
| UC-10 | Datadog の確認 | 組織を選び、コスト、ダッシュボード、メトリクスを表示する。未ログインの組織は OAuth でログインし、使わなくなった組織は CLI (`thief datadog auth logout`) でログアウトする | FR-21、FR-22 |
| UC-11 | 端末上での一覧と加工 | CLI でリソースを一覧し、CSV に出力して他のツールに渡す | FR-10 |
| UC-12 | 表示の調整 | 2 つのサービスを並べて表示し、テーマ、言語、Drawer の位置を切り替える | FR-25、FR-26 |
| UC-13 | backend の起動待ち | frontend だけが先に起動しているときに、接続待ちであることを知る | FR-27 |
| UC-14 | 端末の中での一覧と接続 | ブラウザを開かずに `thief tui` を起動し、プロファイル、リージョン、サービスを切り替えながら AWS のリソースを一覧と Drawer (Overview と Tags) で確認し、EC2 インスタンスか ECS のタスクのシェルに入る | FR-29、FR-30 |

## スコープ

### やること

- AWS の 23 サービス (`frontend/src/lib/serviceMeta.ts` `SERVICES`) の一覧と詳細の閲覧
- Google Cloud の 6 サービス (`GCP_SERVICES`) の一覧と詳細の閲覧
- Datadog のコスト、ダッシュボード、メトリクスの閲覧
- TiDB Cloud のクラスタとコストの閲覧
- 書き込み操作のうち次のもの: S3 と GCS のオブジェクトのアップロードとテキスト編集、Secrets Manager と Parameter Store の値の上書き
- Athena と BigQuery の読み取りクエリの実行と取り消し
- EC2 と ECS への対話セッション
- AWS SSO のログインとログアウト、Datadog の OAuth のログインとログアウト
- CLI (`thief <service> ...`) による一覧、参照、一部の書き込み
- 利用者の端末上で動く API サーバ (`thief server`) と Web UI
- 全画面の TUI (`thief tui`) による AWS のリソースの一覧、Drawer (Overview と Tags)、EC2 と ECS への対話セッション (第 1 段階。FR-29、FR-30)

「機能要求」に挙げた機能は、すべて対象とする。

### やらないこと

- 外部サービスのリソースの作成、削除、設定変更 (FR-5、FR-6、FR-20 に挙げた値とオブジェクトの書き込み、および FR-3 の CloudFront の invalidation の作成を除く)。
  利用者の端末の中のファイルの作成と削除 (FR-18 のスニペット、`thief sso generate-config` による `~/.aws/config` の生成) は、この対象外に含めない。
- 書き込みの SQL の実行 (Athena と BigQuery のクエリは、API サーバでも CLI でも読み取りだけを通す。FR-16)。
- 複数人で共有するサーバとしての運用 (利用者の認証、認可、監査ログを持たない)。
- thief 本体の Docker による起動 (2026-07-18 のコミット cb2da77 で削除した)。
  `example/` のエミュレータ (floci) は docker compose で起動し、これは対象外に含めない。
- ブラウザ以外の GUI のクライアント (CLI と TUI は除く。Flutter 時代の macOS デスクトップ対応は廃止した。`AGENTS.md` の frontend 「基本方針」)。
- TUI の第 1 段階に含めないもの: Google Cloud、Datadog、TiDB Cloud のビュー、分析のビュー (Athena、CloudWatch Logs、BigQuery、Cloud Logging)、リソースの変更操作 (FR-5、FR-6、FR-20 の値とオブジェクトの書き込み、FR-3 の invalidation の作成) は第 2 段階以降で扱う (2026-10-23 の決定。段階ごとの範囲と順序は第 1 段階の完了後に開発者が決める。「未確定論点」の「TUI の第 2 段階以降の範囲」)。
  コストと料金のビュー、オブジェクトのプレビューと SQL 検索、分割表示、TUI の状態の永続化 (最後に使ったプロファイル、リージョン、サービス) は、材料に TUI で扱うかの記述が無く、起草者の整理として第 1 段階では扱わない (扱うかは同じ未確定論点で決める)。
  EC2 と ECS への対話セッション (FR-30) は第 1 段階に含める。
- URL によるルーティング (react-router などを導入しない。`AGENTS.md` の frontend 「基本方針」)。
- EC2 の台数の時系列グラフ (標準で取れるメトリクスが無いため撤回した。issue 0186)。
- Fargate Spot の料金表示 (データ源が無いため保留した。`docs/issues/pending/0059-feat-pricing-fargate-spot-rates.md`)。
- エミュレータ (`example/` の floci) で再現できない機能の、エミュレータ上での動作確認 (SSO ログイン、ターミナル、Cost Explorer と請求系)。

## 機能要求

優先度は材料に記録が無い。
本 PRD では、起草者が次の基準で付け、2026-10-02 に開発者が確認した。
2026-10-23 に追加した FR-29 と FR-30 の Must は、TUI の追加の決定 (「決定した論点」) に基づいて起草者が付けたものであり、開発者の確認は未了である。

- **Must**：欠けると UC の主目的が果たせない機能
- **Should**：UC の主目的は果たせるが、手間が増える機能
- **Could**：補助的な機能と、`docs/issues/TODO.md` で未着手の機能

### AWS リソースの閲覧

**FR-1 AWS のプロファイルとリージョンの選択** (Must)

- `~/.aws/config`、`~/.aws/credentials`、`~/.aws/sso/cache` にあるプロファイルを一覧できる (`backend/internal/aws/profiles.go` `ListProfiles`)。
- 各プロファイルに認証方式 (`sso`、`access_key`、`assume_role`、`credential_process`、`unknown`) と SSO の状態 (`valid`、`expired`、`not_logged_in`) を表示する。
- 複数のプロファイルをタブで開き、切り替えられる。
  開いたタブは再読み込みの後も残る (`PersistedState.awsSessions`)。
- リージョンの選択肢は `DescribeRegions` の結果から作る。
  選んだリージョンは再読み込みの後も残る。

受け入れ基準:

- `~/.aws/config` に `[profile a]` と `[profile b]` を置いて起動すると、`GET /api/aws/profiles` の応答に `a` と `b` が含まれる。
- タブを 2 つ開いて再読み込みすると、同じ 2 つのタブが開いた状態で表示される。
- `^[A-Za-z0-9_\-]+$` に一致しないプロファイル名をパスに置いて `GET /api/aws/profiles/{profile}/identity` を呼ぶと、backend は 400 を返す。
- `~/.aws/credentials` にだけ定義され、`^[A-Za-z0-9_\-]+$` に一致しないプロファイル名は、`GET /api/aws/profiles` の結果に含まれない (`backend/internal/aws/profiles.go`)。
- `~/.aws/config` に定義したプロファイルは、名前が `^[A-Za-z0-9_\-]+$` に一致しなくても (`[profile CT Audit]` など) `GET /api/aws/profiles` の結果に含まれる。

**FR-2 AWS サービスのリソース一覧** (Must)

- サイドバーに 23 サービスを 10 カテゴリで表示する (`AWS_SERVICE_GROUPS`)。
- 従属するリソース (パラメータ、イメージ、ターゲット、イベントなど) は一覧に載せず、FR-3 の Drawer で取得する (`docs/adr/0016-list-free-fields-lazy-drawer.md`)。
- 一覧の列のために項目ごとに追加の API を呼ぶサービス (WAF の Rules と Associated、IAM の MFA、グループ、ポリシー) は、呼び出しの同時実行数を 10 に制限する (`backend/internal/aws/waf.go` `wafACLConcurrency`、`backend/internal/aws/iam.go` `iamDetailConcurrency`)。
  S3 もバケットごとに Region、Public、Encryption の列のための API を呼ぶ (`backend/internal/aws/s3.go`、同時実行数 30)。
- 一覧には状態やリージョンの絞り込み (`FacetBar`) と、件数の集計 (`StatsRow`) を付ける。
- ECS の一覧には、クラスタごとの稼働タスク数 (CloudWatch の `AWS/ECS` 名前空間の `LiveTaskCount`) の時系列グラフを付ける (`GET /api/aws/profiles/{profile}/ecs/timeseries`、`backend/internal/aws/ecs_metrics.go`、issue 0186)。
- サイドバーの件数バッジは、一覧の取得済みの結果だけを表示し、バッジのために取得を起こさない (issue 0187、`frontend/src/hooks/useCachedQueryData.ts`)。
  Google Cloud のサイドバー (FR-20) も、同じ `useCachedQueryData` で件数を読む。
  現在の `frontend/src/views/GcpSidebar.tsx` は `useCachedQueryData` を使わず、TanStack Query の実装の詳細に依存している (「決定した論点」)。

受け入れ基準:

- サイドバーに表示されるサービスの数が 23 である。
- 一覧を開いていないサービスのバッジは、件数を表示しない。
- ECS の一覧を開くと、クラスタごとの稼働タスク数のグラフ (Tasks per cluster) が表示される。

**FR-3 リソースの詳細 (Drawer)** (Must)

- 一覧で行を選ぶと Drawer を開き、Overview と、サービスごとのタブを表示する (`frontend/src/components/Drawer/Drawer.tsx` `DRAWER_TABS`)。
- タブの内容はタブを開いたときに取得する。
- タブの取得が失敗したとき、AWS が返したエラーの本文をそのまま (英語で) 表示する (issue 0075、0082、`frontend/src/components/Drawer/drawerError.tsx`)。
- CloudFront の Drawer から、パスを指定して invalidation を作成できる (`POST /api/aws/profiles/{profile}/cloudfront/{id}/invalidations`)。
  backend の API は実装済みで、画面は未実装である (「決定した論点」)。

受け入れ基準:

- ECS の Drawer に Overview、Services、Tasks、Instances、Terminal、Tags のタブがある。
- CloudFront の Drawer でパス `/*` の invalidation を作成すると、そのディストリビューションの invalidation の一覧 (`aws cloudfront list-invalidations`) に、作成した invalidation が加わる。
- タブを開くまで、そのタブ用の API リクエストが送られない (ブラウザの開発者ツールのネットワーク記録で確認する)。

**FR-4 部分的な取得失敗の表示** (Should)

- 項目ごとに追加の API で取得する値 (WAF の Associated、IAM の MFA、グループ、ポリシー、SQS と WAF と DynamoDB のタグ) の取得に失敗したとき、一覧全体を失敗にせず、その値が取得失敗であることを示す (`*_fetch_failed` フィールド、`backend/internal/aws/sqs.go` `TagsFetchFailed` など)。
- 一覧の列にある値は一覧の行で示し (`frontend/src/components/tables/columns.tsx` `FetchFailedWarning`)、タグは Drawer の Tags タブで示す。

受け入れ基準:

- WAF の `associated_count_fetch_failed` が true の行で、Associated の列が空欄ではなく取得失敗を示す表示になる。
- 取得失敗の行があっても、ほかの行は表示される。

### AWS の書き込み操作

**FR-5 Secrets Manager と Parameter Store の値の閲覧と上書き** (Must)

- 一覧には値を載せない。
  値は Drawer の Value タブを開いたときに取得し、backend はキャッシュしない (issue 0068)。
- 値の上書きは、確認ダイアログで承認したときだけ送る。
- 同時更新の競合検出は行わない。

受け入れ基準:

- `GET /api/aws/profiles/{profile}/secretsmanager` の応答に値のフィールドが無い。
- 確認ダイアログでキャンセルすると、上書きのリクエストが送られない。
- 上書きが成功した後に Value タブを開き直すと、新しい値が表示される。

**FR-6 S3 のオブジェクト操作** (Must)

- バケット内のオブジェクトを prefix 単位で一覧する。
  一覧には階層モードとフラットモードがある (ADR 0029)。
  階層モードは区切り文字 `/` で 1 階層ずつ取得し、フォルダ行とパンくずで辿る。
  フラットモードは今いるフォルダ以下の全階層のオブジェクトを平らに出す。
  1 回の一覧は、オブジェクトとフォルダを合わせた件数が 1000 に達した後にまだ残りがあるときに 1000 件で打ち切り、どちらのモードでも打ち切ったことを画面に示す (`maxS3ListObjects`、応答の `truncated`)。
- オブジェクトをダウンロードできる。
- 5 MiB 未満で、既知のバイナリ拡張子に当たらず、内容が UTF-8 で NUL を含まないオブジェクトをプレビューできる (`backend/internal/api/handlers_object_preview.go`)。
  プレビューできない行はプレビューの操作を無効にし、グレーで表示する。
- 100 MiB 以下のファイルをアップロードできる (`maxS3UploadSize`)。
- プレビューできるオブジェクトはテキストとして編集でき、上書き保存の前に確認ダイアログを出す。

受け入れ基準:

- 階層モードで、フォルダとオブジェクトを合わせて 1500 件あるフォルダを開くと、1000 件を表示し、打ち切りの通知を表示する。
- ちょうど 1000 件のフォルダを開くと、打ち切りの通知を表示しない。
- ちょうど 5 MiB のテキストオブジェクトのプレビューは、413 `PREVIEW_TOO_LARGE` になる。
- 100 MiB を超えるファイルのアップロードは失敗し、オブジェクトは作られない。
- 既知のバイナリ拡張子の一覧は、backend の `previewBinaryExtensions` と frontend の `PREVIEW_BINARY_EXTENSIONS` で同じ集合である。

### 対話セッション

**FR-7 EC2 と ECS への対話セッション** (Must)

- EC2 インスタンスに Session Manager で、ECS のタスクのコンテナに ECS Exec で、ブラウザのターミナルから接続できる。
  ECS の既定のコマンドは `/bin/sh` とする。
- ECS はタスク一覧の行から、コンテナを指定して接続できる (issue 0026)。
- ブラウザ側で接続を閉じると、backend は SSM のセッションを終了する (`TerminateSSMSession`)。
- WebSocket の接続は、`THIEF_WEB_ORIGINS` に列挙した Origin からだけ受け付ける (既定は `localhost:8088` と `127.0.0.1:8088`)。
  Origin とアップグレードの検査は、AWS のセッションの開始 (`StartSession`、`ExecuteCommand`) より前に行う (issue 0217)。

受け入れ基準:

- ターミナルを閉じた後、`aws ssm describe-sessions --state Active` の結果にそのセッションが残らない。
- WebSocket のアップグレードを要求しない GET は 426 で、`THIEF_WEB_ORIGINS` に含まれない Origin からのアップグレードは 403 で、どちらも AWS のセッションを開始せずに拒否される。

**FR-8 ターミナルドック** (Should)

- 開いたターミナルは、サービスやビューを切り替えても接続を保つ。
  セッションが終わるのは、利用者がそのターミナルを閉じたときだけである (issue 0174)。
- ドックの高さをドラッグで変えられ、高さは再読み込みの後も残る。
  タブバーを除く本文の高さは 160px 以上、画面の高さの 0.85 倍からタブバーの高さ (32px) を引いた値以下とする (この上限が 160px を下回る画面では、上限も 160px とする) (issue 0176、`frontend/src/hooks/useTerminalDockHeight.ts`)。

受け入れ基準:

- EC2 のターミナルを開いたまま Google Cloud のビューに切り替えて戻ると、同じセッションが続いている (入力の履歴が残っている)。
- ドックの本文 (タブバーを除く) の高さを 160px 未満にドラッグしても、本文の高さは 160px で止まる。

### 認証

**FR-9 AWS SSO のログインとログアウト** (Must)

- AWS の API が SSO トークンの期限切れで失敗したとき、backend は 401 `SSO_TOKEN_EXPIRED` を返す。
  アクセス拒否は 403 `ACCESS_DENIED` として区別する (issue 0073)。
- frontend は 401 `SSO_TOKEN_EXPIRED` を受けたら再ログインの案内 (`SSOExpiredBanner`) を表示する。
- 再ログインは RFC 8628 のデバイス認可で行う。
  認可 URL はサーバが返した `verification_uri` を使う (issue 0132)。
  ポーリングの間隔、`slow_down` による 5 秒の加算、`expires_in` による打ち切りは RFC 8628 に従う (issue 0129)。
- 取得したトークンは AWS CLI と互換の形式で `~/.aws/sso/cache` に保存する (ディレクトリ 0700、ファイル 0600)。
- ログアウトは `sso:Logout` を呼んで AWS 側のセッションも失効させる (issue 0159)。
- ログイン後、期限の表示を再読み込みなしに更新する (issue 0030)。

受け入れ基準:

- 期限切れのトークンで一覧を開くと、`SSOExpiredBanner` が表示される。
- デバイス認可を完了すると、再読み込みなしに一覧が表示される。
- 認可サーバが `slow_down` を返したとき、次のポーリングまでの間隔が 5 秒増える (backend の単体テストで確認する)。
- ログアウト後、そのプロファイルの SSO のトークンのキャッシュ (`~/.aws/sso/cache` 以下) が削除されている。

### CLI

**FR-10 CLI による一覧と操作** (Must)

- 次の表のコマンドで、各サービスの一覧を表示できる (`backend/internal/cli/`)。

  | サービス | 一覧のコマンド |
  | --- | --- |
  | ec2 | `thief ec2 ls` |
  | ecs | `thief ecs clusters` |
  | rds | `thief rds ls` |
  | elasticache | `thief elasticache ls` |
  | cost | `thief cost ls` |
  | cfn | `thief cfn ls` |
  | iam | `thief iam ls` |
  | ecr | `thief ecr ls` |
  | lambda | `thief lambda` |
  | dynamo | `thief dynamo` |
  | apigw | `thief apigw` |
  | natgw | `thief natgw` |
  | sqs | `thief sqs` |
  | waf | `thief waf` |
  | kinesis | `thief kinesis` |
  | cloudfront | `thief cloudfront` |
  | elb | `thief elb` |
  | logs | `thief logs ls` |
  | s3 | `thief s3 ls` |
  | ssm | `thief ssm param ls` |
  | secretsmanager | `thief secretsmanager ls` |
  | sso | `thief sso ls` |
  | athena | `thief athena workgroups` |
  | gcp | `thief gcp projects ls` |
  | bq | `thief bq dataset ls` |
  | datadog | `thief datadog historical` |
  | tidb | `thief tidb cluster` |

- 共通のフラグとして `-p/--profile`、`-r/--region`、`-o/--output` (`tab` か `csv`)、`--no-header`、`-g/--group-by` を持つ。
- 書き込みのコマンドとして `s3 upload`、`gcp gcs upload`、`ssm param put`、`secretsmanager put`、`ecs exec`、`ec2 session`、`sso login`、`sso logout`、`datadog auth login`、`datadog auth logout`、`datadog auth refresh`、`sso generate-config` を持つ。
- 参照のコマンドとして `athena query`、`bq query`、`bq table info`、`logs events`、`s3 objects`、`s3 download`、`gcp gcs download`、`ssm param get`、`secretsmanager get`、`ecr images`、`rds parameters`、`elasticache parameters`、`ecs services`、`ecs tasks`、`rds cluster`、`rds cluster-parameters`、`athena catalogs`、`athena databases`、`athena tables`、`gcp run ls`、`gcp iam ls`、`gcp serviceaccounts ls`、`gcp logging ls`、`gcp gcs ls`、`gcp gcs objects`、`gcp projects refresh`、`bq table ls`、`tidb project`、`tidb cost`、`datadog estimated`、`cost service`、`cost account`、`cost usage-type`、`cost overview`、`elb listeners`、`elb rules`、`elb target-groups`、`elb target-health`、`cfn describe`、`cfn changeset`、`cost forecast` を持つ。
- `thief server` で API サーバを起動する。
- 上記の列挙は 2026-09-26 時点の `backend/internal/cli/` のコマンド (テスト用のものを除く) をすべて挙げたものである。
- 2026-10-23 に、全画面の TUI を起動する `thief tui` を加えた (FR-29)。
- SIGINT か SIGTERM を受けたら処理を取り消し、終了コード 130 で終わる (`backend/internal/cli/run.go` `interruptExitCode`)。
  `thief tui` は、`q`、画面で押した Ctrl-C、SIGTERM では終了コード 0 で終わり、プロセスの外から送られた SIGINT (`kill -INT`) だけが 130 になる (FR-29)。
- 設定は、コマンドライン引数、環境変数、設定ファイル、既定値の順で優先する。

受け入れ基準:

- `thief ec2 ls -o csv` の出力がカンマ区切りで、1 行目がヘッダである。
- `thief ec2 ls -o csv --no-header` の出力にヘッダ行が無い。
- 一覧の実行中に Ctrl-C を押すと、終了コードが 130 になる。
- 環境変数 `THIEF_REGION` と `-r` の両方を指定すると、`-r` の値が使われる。

### TUI

**FR-29 全画面の TUI による AWS のリソースの一覧と詳細** (Must)

- `thief tui` で、端末の中で動く全画面の TUI を起動する (`backend/internal/tui/`)。
  共通のフラグ `-p/--profile` と `-r/--region` で、起動時のプロファイルとリージョンを指定できる。
- TUI は API サーバ (`thief server`) を経由せず、CLI と同じ取得層 (`backend/internal/aws/`) をプロセス内で直接呼ぶ (ADR 0033)。
  利用者は TUI のために API サーバとブラウザを起動しなくてよい。
- 第 1 段階の対象サービスは、Web UI の AWS のサイドバー (FR-2) にある 23 サービスのうち、一覧のビューを持つ 19 サービスとする。
  ID と名前は、`ec2` (EC2)、`lambda` (Lambda)、`ecr` (ECR)、`ecs` (ECS)、`s3` (S3)、`rds` (RDS)、`dynamo` (DynamoDB)、`cache` (ElastiCache)、`elb` (ELB)、`cloudfront` (CloudFront)、`apigw` (API Gateway)、`natgw` (NAT Gateway)、`kinesis` (Kinesis)、`sqs` (SQS)、`iam` (IAM)、`waf` (WAF)、`secrets` (Secrets Manager)、`ssm` (Parameter Store)、`cfn` (CloudFormation) である (ID は `frontend/src/lib/serviceMeta.ts` の `SERVICE_TO_PATH` のキー、名前は同じファイルの `SERVICES` の `name`)。
  2026-10-23 時点のプロトタイプは `ec2` だけを持ち、残りの 18 サービスは issue 0219 から 0223 で足して 2026-10-26 時点で 19 サービスが揃っている (「決定した論点」)。
  専用のビューを持つ 4 サービス (Athena、CloudWatch Logs、Cost Explorer、Pricing) は対象に含めない (「スコープ」)。
  サービスは `S` のキーで開く選択肢から切り替える。
- 画面は、ヘッダ (サービス名、プロファイル、リージョン、取得中の表示)、一覧、Drawer、通知行、キー操作のヒントの行からなる。
  一覧と Drawer は左右に並べ、`tab` で操作の対象を切り替える。
- 一覧の列は、Web UI でそのサービスを選んだときに表示する一覧 (`frontend/src/views/AccountView.tsx` が `ServicePanel` に渡す `frontend/src/components/tables/columns.tsx` の列定義) と同じ項目とする。
  列の幅は見出しと表示中の値の最大幅から決め、端末の幅に収まらないときは広い列から縮める。
- `/` で絞り込みの入力を開き、入力した文字列を含む行 (すべての列の値を対象とする。大文字と小文字を区別しない) だけを表示する。
- 一覧で選んだ行の Drawer に、Overview と Tags を表示する。
  Overview は、Name と State に続けて、Web UI の Drawer の Overview と同じ項目 (`frontend/src/components/Drawer/overviewRows.tsx`) を並べる。
  Tags は、Web UI の Drawer の Tags タブと同じキーと値を表示し、タグが無い行では `No tags` と表示する。
  Web UI が一覧とは別の取得でタグを得るサービス (CloudFormation は `frontend/src/components/Drawer/DrawerCFNTags.tsx` がスタックの詳細 `backend/internal/aws/cfn.go` `DescribeCFNStackDetail` から取る) では、TUI も行を選んだときに同じ取得関数でタグを取る。
  Web UI のサービス固有のタブ (ECS の Services、Tasks など) を TUI の Drawer に置くかは、第 1 段階では決めない (「未確定論点」)。
- プロファイル (`p`)、リージョン (`R`)、サービス (`S`) の選択肢は、文字を打つだけで名前と説明の部分一致で絞り込める。
  矢印キーか `ctrl+n` / `ctrl+p` で移動し、`enter` で決める。
  プロファイルの選択肢には、SSO のプロファイルではアカウント ID、ロール名、SSO の状態 (FR-1 と同じ `valid`、`expired`、`not_logged_in`) を名前に添えて表示する。
  他のプロファイルは名前だけを表示する (`backend/internal/aws/profiles.go` の `Profile.AccountID` は SSO のプロファイルにしか無い)。
  リージョンの選択肢は FR-1 と同じ `DescribeRegions` の結果から作る。
- `r` で表示中のサービスを取得し直す。
  TUI はリソースの一覧の取得結果をキャッシュせず、プロファイル、リージョン、サービスを切り替えるたびに取得する。
  プロファイルの一覧は起動時に 1 回取得する。
  リソースの一覧の取得に失敗したときは、失敗の内容を一覧の領域に表示し、次の取得を始めるまで残す。
- SSO トークンの期限切れは、FR-9 と同じ判定 (`backend/internal/aws/errors.go` `IsSSOTokenExpired`) でバナーに表示し、`thief sso login` の実行を案内する。
  TUI の中でデバイス認可を行うかは、第 1 段階では決めない (「未確定論点」)。
- 通知 (セッションの終了と失敗、プロファイルとリージョンの一覧の取得の失敗、セッションを開始できない行の選択) はキー操作のヒントの行の上に表示し、6 秒が経つかいずれかのキーを押すと消える。
  通知は 1 行で表示し、ヒントの行を隠さない。
  失敗の内容が改行を含むとき (`errors.Join` で複数の失敗をまとめたセッションの失敗など) は、改行を空白に置き換える。
- `q` か `ctrl+c` で終了する (対話セッションの中を除く。FR-30)。
  絞り込みの入力中と選択肢の表示中は `q` を入力の文字として扱い、`ctrl+c` だけで終了する。
  `q`、画面で押した Ctrl-C (raw モードのキー入力として届く)、SIGTERM (bubbletea が `QuitMsg` に変換する) では終了コードは 0 であり、CLI の中断の終了コード 130 (FR-10) を適用しない (ADR 0033)。
  プロセスの外から送られた SIGINT (`kill -INT`) は bubbletea が中断として扱い、CLI と同じく 130 で終わる。

受け入れ基準:

- `thief tui --help` の終了コードが 0 で、出力の `Usage:` の次の行が、前後の空白を除いたとき `thief tui` で始まる。
- 19 サービスそれぞれについて、`Service.Columns()` の見出しの並びが Web UI の列定義の `header` の並びと一致し、`Row.Overview()` のキーの並びが `overviewRows.tsx` の同じサービスの項目名から `Name` と `State` を除いた並びと一致し (`Name` と `State` は Drawer の描画 `refreshDrawer` が先頭に置く)、`Row.State()` と State の列が Web UI の `StatusBadge` が表示する値 (資源の `State` フィールドの生の値) と一致し、`Cells()` と `Overview()` の値が Web UI の同じセルと項目が描画する文字列と一致し (空の値を `—` にするのは Web UI が `Dash` か `dash` を描画する項目に限る。項目ごとの取得失敗 (IAM の MFA、Policies、Groups と WAF の Associated) は、Web UI の警告アイコンに代えて `fetch failed` の文字列を値に付ける)、`Row.Tags()` が Web UI の Drawer の Tags タブと同じキーと値を返す (CloudFormation は一覧でタグを取れないため、`Row.Tags()` ではなく行を選んだときに取得するタグが Web UI の Tags タブと一致する)。
- 19 サービスそれぞれについて、`Service` を作る関数に偽の一覧関数を渡して `Load` を呼ぶとその関数が呼ばれることを単体テストで確認し、`tui.Deps` を組み立てる関数が各 `Service` に渡す一覧関数が、Web UI の同じ一覧のハンドラ (`backend/internal/api/handlers_aws.go`) が呼ぶ `backend/internal/aws/` の関数と同じであることをコードの読み合わせで確認する。
- 改行を含む失敗の内容を通知に渡したとき、`View()` の出力で通知が 1 行であり、キー操作のヒントの行が最後の行に表示されることを単体テストで確認する。
- `mise run backend:install` で `thief` を `$(go env GOPATH)/bin` に導入し (以下の `thief` はそのパスのもの)、`example/` の floci を起動し (`mise run example:up`)、`example/seed.sh` と同じダミーの認証情報とリージョン (`AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=ap-northeast-1`) で `aws --endpoint-url http://localhost:4566 ec2 run-instances --image-id ami-12345678 --instance-type t3.micro --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=web-01}]'` と、Name を `batch-01` にした同じコマンドを実行して EC2 インスタンスを 2 台作ってから、リポジトリのルートで `HOME="$(pwd)/example/home" thief tui -p floci` を起動すると、一覧に `web-01` と `batch-01` の行が表示される (`example/seed.sh` は EC2 インスタンスを作らない。この手順は `example/README.md` の「確認手順」に記録している。`thief tui` の認証情報は `example/home/.aws/credentials` の `floci` を使う)。
  選んだ行の Drawer の Overview に Name、State、Resource ID、Type、AZ、Region、Private IP、Public IP、VPC、Uptime、Launched が表示される。
  Drawer の Tags に `Name` と `web-01` が表示される。
- 上の環境で `/` を押して `web` と入力すると、`web-01` の 1 行だけが表示される。
- 上の環境で `p` を押して `flo` と入力すると、選択肢が `floci` だけになり、件数が `(1/1)` と表示される (`example/home/.aws/config` のプロファイルは `floci` だけである)。
- 実 AWS の、認証情報が無効なプロファイルに `p` で切り替えると、一覧の領域に `list EC2 instances:` で始まる失敗が表示され、通知行には何も表示されない。
  続けて `R` を押すと、通知行に `list regions:` で始まる失敗が表示され、キー操作のヒントの行は表示されたままである。
- 通知を表示してから 6 秒後に、通知行が消えている。
- 実 AWS の SSO プロファイルで、`~/.aws/sso/cache/` の該当するトークンの `expiresAt` を過去の時刻に書き換え、`refreshToken` のキーがあれば削除してから起動すると (SDK は `clientId`、`clientSecret`、`refreshToken` が揃うとトークンを更新する。`github.com/aws/aws-sdk-go-v2/credentials/ssocreds` の `SSOTokenProvider`)、一覧の取得が失敗し、バナーに `thief sso login` が表示される。
- floci の環境で `mise run example:down` で floci を止めてから `r` を押すと、一覧の領域に失敗が表示される。
  `mise run example:up` で floci を起動し直してから `r` を押すと、失敗の表示が消えて取得中の表示になる。
- `q` で終了したとき、画面で Ctrl-C を押して終了したとき、`kill -TERM <pid>` で終了したときのプロセスの終了コードが 0 である。
- `kill -INT <pid>` で終了したときのプロセスの終了コードが 130 である。
- `thief tui` のプロセスは TCP の待ち受けを持たない (`lsof -nP -a -p <pid> -iTCP -sTCP:LISTEN` の出力が空である)。
- `backend/internal/tui/` は `backend/internal/api` に依存しない (`backend/` で実行した `go list -deps ./internal/tui/` の出力に `internal/api` が含まれない)。

**FR-30 TUI からの対話セッション** (Must)

- EC2 の一覧で選んだインスタンスに、`s` で Session Manager のセッションを開始する。
- ECS では、クラスタを選び、タスクとコンテナを選んで ECS Exec で接続する。
  既定のコマンドは FR-7 と同じ `/bin/sh` とする。
  タスクとコンテナは、クラスタ行で `s` を押した後に、絞り込み付きの選択肢で順に選ぶ (「決定した論点」の「TUI の ECS のタスクとコンテナの選び方」)。
  候補が 1 件のときは選択肢を出さずに自動で選ぶ。候補は実行中のタスクと、ECS Exec が有効なコンテナに限る。
- セッションは TUI のプロセスの中で `backend/internal/session` の DataChannel を使って行う。TUI は `session-manager-plugin` を起動しない。
  EC2 は `backend/internal/aws/ssm_session.go` の `StartSSMSession`、ECS は `backend/internal/aws/ecs_exec.go` の `ExecuteECSCommand` でセッションを開始し、`session.OpenDataChannel` でデータチャネルを開く。
- セッションは TUI の本文の中央にフロートウィンドウとして表示する。TUI の一覧と Drawer はウィンドウの背後に残り、セッションの間も TUI の代替画面を離れない。
  ウィンドウはユーザーによる移動とリサイズができない。端末のリサイズには追従し、端末画面の大きさを `SendSize` でリモートへ送る。
  本文が 50 桁 x 18 行未満のときはセッションを開始しない。
- セッション中は `Ctrl-]` だけがウィンドウを閉じる。それ以外のキー (`Ctrl-C`、`q`、通常文字、矢印、Tab などを含む) はリモートへ送る。
  `Ctrl-C` はリモートのプロセスに渡り、TUI を終了させない。`q` もリモートへ送る。
- `Ctrl-]` でウィンドウを閉じたとき、TUI を `SIGTERM` や `kill -INT` で終了したとき、リモートの `exit` でデータチャネルが閉じたときは、AWS 側のセッションを `TerminateSSMSession` で終える (FR-7 と同じ)。
  リモートの `exit` では最後の画面と `Session closed` を残し、`Ctrl-]` でウィンドウを閉じる。
  通知行には成功時に `session closed`、失敗時に `session closed with error: <理由>` を表示する。
- フロートウィンドウは同時に 1 つだけ開く。2 つ目のセッションは、1 つ目を `Ctrl-]` で閉じるまで開始しない。

受け入れ基準:

- 実 AWS アカウントの、SSM Agent が動く EC2 インスタンスの行を選んで `s` を押すと、TUI の本文の中央にフロートウィンドウが開き、シェルで `echo ok` を実行すると `ok` が返る (`example/` のエミュレータでは確認できない)。
  ウィンドウの中の `Ctrl-C` と `q` はリモートへ送られ、TUI は終了しない。
- `Ctrl-]` でウィンドウを閉じた後、`aws ssm describe-sessions --state Active` の結果にそのセッションが残らない。
- リモートで `exit` を実行すると、最後の画面と `Session closed` が残り、通知行に `session closed` が表示される。`Ctrl-]` でウィンドウが閉じる。
- 実 AWS アカウントの、ECS Exec を有効にしたタスクを持つクラスタの行を選んで `s` を押し、タスクとコンテナの選択肢で接続先を選ぶと、TUI のフロートウィンドウにシェルが表示され、`echo ok` を実行すると `ok` が返る (`example/` のエミュレータでは確認できない)。
  `Ctrl-]` の後、`aws ssm describe-sessions --state Active` の結果にそのセッションが残らない。
- 端末の大きさを変えると、リモートの端末の桁数と行数が新しいウィンドウの内側の大きさに追従する。
- 本文が 50 桁 x 18 行未満のとき、`s` を押してもセッションを開始せず、通知行に `terminal too small for a session window` が表示される。
- `session-manager-plugin` を PATH から外した状態でも、TUI の EC2 Session Manager と ECS Exec のセッションが開始できる。CLI の `thief ec2 session` と `thief ecs exec` は引き続きプラグインを必要とする。

### 分析とコスト

**FR-11 AWS Cost Explorer** (Must)

- 指定した期間のコストを、日単位か月単位で表示する。
  期間は日付の指定と、直近 1 週間、1 か月、3 か月、6 か月のプリセットで選ぶ。
- 集計の軸はサービス、使用タイプ、リンクアカウント、リージョンから選ぶ。
  指標は UnblendedCost と NetAmortizedCost を切り替える。
- キーワードで、サービス、使用タイプ、リンクアカウントを横断して絞り込める。
- グラフの系列は金額の大きい上位 8 本とし、残りは Other の 1 本にまとめる (`frontend/src/views/CostExplorerPanel.tsx` `MAX_SERIES`)。
- 表示中の集計表を CSV で出力できる。
- Cost Explorer の API は us-east-1 に対して呼ぶ。
- コストの予測を、Web の画面と CLI (`thief cost forecast`) で表示する。
  Web の画面での予測の表示は未実装である (`frontend/src/api/queries.ts` `useCostForecast` を画面から使う箇所が無い。「決定した論点」)。

受け入れ基準:

- 10 サービス分のコストがある期間を開くと、グラフの系列がちょうど 9 本 (上位 8 本と Other) である。
- CSV 出力の行数が、画面の集計表の行数と一致する。
- Web の画面に、当月のコストの予測額が表示される。

**FR-12 AWS の料金表示 (Pricing)** (Must)

- 次の 8 種の料金を表示する: EC2、EC2 Spot、ECS (Fargate)、RDS、ElastiCache、Compute Savings Plans、EC2 Instance Savings Plans、Database Savings Plans。
- リザーブドインスタンスの料金は EC2、RDS、ElastiCache だけに表示する。
- 料金の表を、インスタンスタイプの文字列とインスタンスファミリで絞り込める。
  ファミリの絞り込みは EC2、EC2 Spot、RDS、ElastiCache と 3 種の Savings Plans で使える。
  2 つの絞り込みは AND で効く (issue 0054、`frontend/src/lib/pricingAttributeFilters.ts`)。
- 料金の行を選び、行ごとに数量を指定して、選んだ行の月額の見積もりを出せる。
  選んだ行と数量は再読み込みの後も残る (`frontend/src/lib/pricingSelection.ts`、`PricingPersistedState`)。
- 月額の見積もりは 730 時間で計算する。
  RI と Savings Plans の見積もりには期間と購入方式を表示する (issue 0051)。
- EC2 Spot の料金は表示のたびに取得する。
  それ以外の料金は取得結果をファイルにキャッシュし、期限を設けない (`THIEF_PRICE_CACHE_DIR`)。
- 60 行以上の料金のグループは、表示範囲にある行だけを描画する (issue 0061、`frontend/src/components/pricing/RateGroupSection.tsx` `WINDOW_THRESHOLD`)。

受け入れ基準:

- EC2 の m5.large の月額の見積もりが、時間単価 × 730 と一致する。
- ECS (Fargate) など RI の無い種別で、RI の料金が表示されない。
- EC2 の料金の表でファミリ `m5` を選ぶと、表示される行のインスタンスタイプがすべて `m5.` で始まる。
- フィルタを入力せずに EC2 の On-Demand (約 780 行、issue 0061 の実測) を開いたとき、DOM に描画された行数が全行数より少ない。
- EC2 の料金のキャッシュのファイルがある状態で、端末をネットワークから切り離して backend を再起動し、ブラウザを再読み込みしてから EC2 の料金を開くと、料金が表示される。

**FR-13 Athena** (Must)

- カタログ、データベース、テーブル、ワークグループを選んでクエリを実行し、結果を表示する。
- Web の画面は結果を 1 ページ 500 行で取得し、利用者が続きを読み込む限り行数の上限を設けない。
  CLI の `thief athena query` は全ページを取得し、行数の上限を設けない (「決定した論点」)。
- 実行中のクエリを取り消せる。
- 実行の履歴を最大 50 件表示する。
- 実行状態は、Web の画面では 1 秒ごと、CLI では 2 秒ごとに取得する (`frontend/src/api/queries.ts` `QUERY_POLL_INTERVAL`、`backend/internal/aws/athena.go` `athenaQueryPollInterval`)。

受け入れ基準:

- `SELECT` を実行すると結果が表示される。
- `DROP TABLE x` を実行すると、クエリは Athena に送られずエラーになる (FR-16)。
- 実行中に取り消すと、Athena 上のクエリの状態が CANCELLED になる。

**FR-14 CloudWatch Logs** (Must)

- ロググループを選び、期間とフィルタでログイベントを検索する。
  Web の画面は 1 回の取得で最大 200 件を要求し、続きを取得できる (`frontend/src/api/queries.ts` `CW_LOG_EVENT_PAGE_SIZE`)。
  件数を指定しないときの backend の既定はロググループあたり 100 件である (`backend/internal/aws/cloudwatchlogs.go` `defaultLogEventPerGroupLimit`)。
- Live Tail でイベントを表示し続けられる。

受け入れ基準:

- 250 件のイベントがある期間を Web の画面で検索すると 200 件を表示し、続きを取得すると残りの 50 件を表示する。
- Live Tail の開始後に書き込まれたイベントが、再読み込みなしに表示される。

**FR-15 BigQuery** (Must)

- データセット、テーブル、スキーマを表示する。
- クエリの実行前にドライランで処理量を表示する。
- 結果は 1 ページ 500 行で取得する。
  実行中のクエリを取り消せる。
  履歴を表示する。

受け入れ基準:

- ドライランの結果として処理されるバイト数が表示される。
- `DELETE FROM x WHERE true` を実行すると、クエリは BigQuery に送られずエラーになる (FR-16)。

**FR-16 読み取り専用の SQL 検査** (Must)

- Athena と BigQuery のクエリは、コメントと文字列リテラルを除いた部分に INSERT、UPDATE、DELETE、MERGE、CREATE、DROP、ALTER、TRUNCATE、GRANT、REVOKE、UNLOAD のいずれかの語を含むとき拒否する (`backend/internal/sqlguard/sqlguard.go`)。
- この検査は、API サーバを通るすべてのクエリと、CLI の `thief athena query` および `thief bq query` に掛ける。
  書き込みを許すフラグは設けない。
- 現在の実装は次の 2 点でこの要求と異なり、どちらも実装を要求に合わせる (「決定した論点」)。
  - コメントと文字列リテラルの中の語でも拒否する。
  - `thief bq query` は検査を通さずに任意の SQL を実行する (`backend/internal/bigquery/query.go` `ExecuteQueryUnrestricted`)。

受け入れ基準:

- API の Athena と BigQuery のクエリの実行、`thief athena query`、`thief bq query` で、上記 11 語のそれぞれを含む SQL が、大文字小文字を問わず拒否される。
- `SELECT 'drop' AS x` と `SELECT 1 -- drop` は拒否されない。
- `/* x */ DROP TABLE t` は拒否される。

**FR-17 オブジェクトの SQL 検索** (Should)

- S3 と GCS のオブジェクトをブラウザ内の DuckDB Wasm で SQL 検索できる。
  対象は csv、tsv、json、jsonl、ndjson とそれぞれの gzip 圧縮、および parquet の 11 形式である (`frontend/src/lib/objectQuery.ts`)。
- 一覧で選んだ複数のオブジェクトを 1 回の検索に掛けられる (ADR 0030)。
  選んだ全オブジェクトが 1 つの `obj` ビューになり、由来のオブジェクトのキーが `object_key` 列に入る。
  組み合わせられるのは同じ形式のオブジェクトだけで、1 回に選べるのは 50 件までとする (`OBJECT_QUERY_MAX_FILES`)。
- 1 回の検索で取り込むオブジェクトの合計サイズの上限は既定 1 GiB とし、`THIEF_OBJECT_QUERY_MAX_BYTES` で変えられる。
  上限は `GET /api/config` で frontend に渡す。
- 結果は 10000 行で打ち切る (`OBJECT_QUERY_MAX_ROWS`)。
- 結果をグラフで表示できる。
  カテゴリは 5000 件までとする (issue 0197)。
- DuckDB の拡張は外部の配布元から取得せず、同じオリジンから配信する。

受け入れ基準:

- 20000 行の CSV に `SELECT *` を実行すると、10000 行を表示し、打ち切りを示す。
- 同じ形式の 3 オブジェクトを選んで `SELECT object_key, count(*) FROM obj GROUP BY object_key` を実行すると、3 行が返り、各行の `object_key` が選んだオブジェクトのキーである。
- 上限を超える大きさのオブジェクトでは、検索の操作を実行できない。
- csv と parquet のオブジェクトを同時に選ぶと、選択に対する検索の操作を実行できない。
- 合計が上限を超える大きさのオブジェクトの組では、選択に対する検索の操作を実行できない。
- 検索の実行中、ブラウザから DuckDB の拡張の外部配布元へのリクエストが発生しない。

**FR-18 保存クエリ (スニペット)** (Could)

- Athena と BigQuery のクエリに名前を付けて保存し、呼び出し、削除できる (`DELETE /api/snippets/{service}/{name}`)。
- 保存先は既定でカレントディレクトリの `.thief/snippets` とし、`THIEF_SNIPPETS_DIR` で変えられる。
- 名前はパーセントエンコード後に 128 バイト以下とする。

受け入れ基準:

- 保存したスニペットが、backend の再起動の後も一覧に表示される。
- 削除したスニペットが、一覧に表示されなくなり、保存先のファイルも無くなる。
- パーセントエンコード後に 129 バイトになる名前の保存は、400 になる。

### Google Cloud

**FR-19 Cloud Logging** (Must)

- 期間のプリセットか任意の期間と、フィルタでログを検索する。
  Web の画面は 1 回の取得で 200 件を要求する (`frontend/src/api/queries.ts` `GCP_LOG_ENTRY_PAGE_SIZE`)。
  件数を指定しないときの backend の既定は 100 件である (`backend/internal/gcp/logging.go` `defaultLogEntryPageSize`)。
- Live Tail でログを表示し続けられる。
- ペインの幅を超える長さの行は折り返さず、横にスクロールして読める。
  指定したフィールドを行の先頭に表示できる。

受け入れ基準:

- フィールドを指定すると、各行の先頭にそのフィールドの値が表示される。
- ペインの幅を超える長さの行があるとき、その行は折り返されず、ログの一覧に横のスクロールバーが表示される。

**FR-20 Google Cloud のリソース閲覧** (Must)

- Application Default Credentials (ADC) で認証する。
- プロジェクトを一覧してタブで開く。
  プロジェクトの一覧は既定では `~/.config/thief/gcp-projects.json` (`XDG_CONFIG_HOME` を設定したときは `$XDG_CONFIG_HOME/thief/gcp-projects.json`) にキャッシュし、初回と明示の更新のときだけ取得し直す。
- Cloud Run (サービスとジョブ)、GCS (バケットとオブジェクト)、IAM (メンバー単位のバインディング)、サービスアカウント、BigQuery、Cloud Logging を表示する。
- GCS のオブジェクトには、FR-6 と同じ規則でダウンロード、プレビュー、アップロード、編集、階層の表示、1000 件の打ち切りを適用する。
- API が有効化されていないとき、403 `GCP_API_DISABLED` を返し、画面にその旨を表示する。

受け入れ基準:

- Cloud Run の API を無効にしたプロジェクトで Cloud Run を開くと、`GCP_API_DISABLED` のエラーが表示される。
- プロジェクトを指定せず、`GOOGLE_CLOUD_PROJECT` も未設定のとき、backend は 503 `GCP_NOT_CONFIGURED` を返す。

### Datadog

**FR-21 Datadog の認証** (Must)

- OAuth 2.0 の Authorization Code (RFC 6749)、PKCE (RFC 7636)、Dynamic Client Registration (RFC 7591) でログインする。
  要求するスコープは `usage_read` とする。
- トークンは `~/.config/thief/datadog/` にファイル権限 0600 で保存する。
- 親組織は、OAuth のトークンが使えないとき (無い、ファイルが壊れている、期限切れで更新に失敗した)、静的キー (`DATADOG_API_KEY` と `DATADOG_APP_KEY`) に切り替える。
  そのトークンでの呼び出しが 403 を返したときも、静的キーで 1 回だけやり直す。
  Sub Organization は静的キーに切り替えない。
- CLI のログインは `127.0.0.1:8400/callback` で認可コードを受け取る。
  ポートが使用中ならエラーにする。
- 組織ごとにログアウトできる。
  Web の画面にはログアウトの操作が無く、CLI と API で行う。
  ログアウトは、その組織の保存したトークンとクライアント登録をローカルから削除する。
  Datadog 側のトークンは失効させない (`backend/internal/datadogauth/login.go` `Logout`、`POST /api/datadog/auth/logout`、`thief datadog auth logout`)。
- 認証情報が無いとき、backend は 401 `DATADOG_NO_CREDENTIALS` を返し、frontend は再ログインの案内を表示する。

受け入れ基準:

- ログイン後、`~/.config/thief/datadog/` のトークンファイルの権限が 0600 である。
- ログアウト後、その組織のトークンファイルが削除され、ほかの組織のトークンファイルは残る。
- OAuth のトークンが無く静的キーがあるとき、親組織のコストが表示される。
- 同じ条件で Sub Organization を開くと、401 `DATADOG_NO_CREDENTIALS` になる。

**FR-22 Datadog のコスト、ダッシュボード、メトリクス** (Must)

- 組織をタブで開き、月単位の実績コストと当月か前月の推定コストを表示する。
- ダッシュボードを一覧し、timeseries と query_value のウィジェットを描画する。
  それ以外の種別は種別名だけを表示する。
- メトリクスのクエリを実行してグラフを描画する。
  Datadog がクエリの誤りを HTTP 200 で返したときもエラーとして表示する。

受け入れ基準:

- 未対応の種別のウィジェットがあるダッシュボードで、そのウィジェットの位置に種別名が表示される。
- 誤ったメトリクスのクエリを実行すると、エラーが表示される。

### TiDB Cloud

**FR-23 TiDB Cloud の認証** (Must)

- API キー (`TIDB_PUBLIC_KEY` と `TIDB_PRIVATE_KEY`) を使い、Digest 認証 (RFC 2617) で認証する。

受け入れ基準:

- 正しいキーを設定すると、`GET /api/tidb/projects` がプロジェクトの一覧を返す。

**FR-24 TiDB Cloud のクラスタとコスト** (Must)

- プロジェクトを選び、クラスタの一覧と月単位のコストを表示する。

受け入れ基準:

- 請求月を指定すると、その月のコストが表示される。

### UI と永続化

**FR-25 分割表示** (Should)

- AWS と Google Cloud のビューで、2 つのサービスを左右に並べて表示できる。
  選択中の行、絞り込み、Drawer のタブはペインごとに持つ (issue 0175)。

受け入れ基準:

- 左のペインで EC2、右のペインで S3 を開き、それぞれで別の行を選ぶと、2 つの Drawer が同時に表示される。

**FR-26 表示設定と永続化** (Must)

- テーマ、アクセントカラー、言語 (日本語と英語)、Drawer の位置を切り替えられる (`frontend/src/hooks/useTweaks.ts`)。
- 表示設定、開いているタブ、選択中のビューとリージョンを、ブラウザの `localStorage` のキー `cloudlens:v1` に保存し、再読み込みの後に復元する。
- 分析のビュー (Athena、CloudWatch Logs、BigQuery、Cloud Logging) のリソースバーの幅と、Drawer の大きさをドラッグで変えられ、再読み込みの後も残る (issue 0042、`PersistedState.resourcePanelWidth`、`frontend/src/components/Drawer/Drawer.tsx` `DRAWER_SIZE_KEY`)。
- Drawer のタブ名と、AWS の API が返した英語のエラーメッセージは翻訳せず、英語で表示する (issue 0066、0075、0082)。

受け入れ基準:

- 言語を英語にして再読み込みすると、英語で表示される。
- リソースバーの幅を変えて再読み込みすると、変えた幅で表示される。
- Drawer のタブ名が、言語を日本語にしても英語のまま表示される。

**FR-27 backend の起動待ちの表示** (Should)

- frontend が backend に接続できないとき、接続待ちの画面 (`ConnectionWaiting`) を表示し、`GET /api/health` を 2 秒ごとに試す。
  接続できたら通常の画面に切り替える。
- 一度 `GET /api/health` が成功した後は health を取得し直さないため、その後のリソースの取得の失敗では接続待ちの画面に戻らない。

受け入れ基準:

- backend を止めた状態で frontend を開くと接続待ちの画面が表示され、backend を起動すると再読み込みなしに通常の画面に切り替わる。

**FR-28 キャッシュの破棄と再取得 (Refresh)** (Should)

- トップバーの Refresh で、表示中のビュー (AWS、Google Cloud、Datadog、TiDB Cloud のいずれか) の backend のキャッシュを破棄してから、画面のデータを取得し直す (`frontend/src/lib/refreshView.ts`、`POST /api/cache/invalidate`)。
- コスト (`cost`、`cost-forecast`)、リージョンの一覧 (`regions`)、Google Cloud のプロジェクトの一覧 (`gcp-projects`)、DynamoDB の Items (`dynamo-items`) のキャッシュは破棄しない (`backend/internal/api/handlers_cache.go` `cacheInvalidateExcluded`)。

受け入れ基準:

- AWS のビューで EC2 の一覧を表示して Refresh を押すと、その後の一覧の取得で AWS の API が呼ばれる (AWS CloudTrail のイベントで確認する)。
- AWS のビューで Refresh を押しても、Google Cloud のキャッシュは破棄されない。

## 非機能要求

### 性能

- backend はリソースの取得結果をメモリにキャッシュする。
  期限は 1 時間、リージョンの一覧と Google Cloud のプロジェクトの一覧は 24 時間、CloudFormation のスタックのイベントは 30 秒とする (`backend/internal/api/server.go` `cacheTTL`、`regionsCacheTTL`、`backend/internal/api/handlers_gcp.go`、`backend/internal/api/handlers_aws.go` `cfnEventsCacheTTL`)。
  Cost Explorer のコストと予測も同じメモリのキャッシュ (期限 1 時間) に載るが、Refresh では破棄しない (FR-28)。
  EC2 Spot を除く料金 (FR-12) は、このメモリのキャッシュではなく、期限の無いファイルのキャッシュに置く (`backend/internal/pricecache/pricecache.go`)。
  EC2 Spot の料金はどちらのキャッシュにも置かず、表示のたびに取得する (`backend/internal/api/handlers_pricing.go`)。
- frontend は取得結果を 60 秒間、新しいものとして扱う (`staleTime`)。
- TUI はリソースの一覧の取得結果をキャッシュしない (FR-29)。
- 外部 API の並列呼び出しは、同時実行数を上限付きにする。
  上限は既定 30、WAF と IAM は 10 とする。
- Cloud Run の一覧は、ロケーションごとのジョブの取得を並列にする。
  43 ロケーションで、一覧全体が 44.9 秒から 6.3 秒に (ロケーションごとのジョブの取得は 43.4 秒から 4.8 秒に) 短くなった実測がある (issue 0043)。
- 大きな結果は件数の上限で打ち切る: オブジェクトの一覧 1000 件、オブジェクトの SQL 検索 10000 行。
- 性能の目標値 (応答時間の上限など) は置かない。
  遅さを計測で確かめた時点で、その計測を根拠に issue を起票する (「決定した論点」)。

### 可用性

- thief は利用者の端末で動く単一のプロセスである。
  冗長化と常時稼働は対象外とする。
- 外部 API の一部の取得失敗で、一覧全体を失敗にしない (FR-4)。
- backend の停止中も frontend は接続待ちを表示する (FR-27)。
- backend は SIGINT と SIGTERM で待ち受けを止め、処理中のリクエストの完了を最大 5 秒待つ (`backend/internal/cli/server.go` `serverShutdownTimeout`)。

### セキュリティ

- API サーバは既定で `127.0.0.1:8089` で待ち受ける (`THIEF_LISTEN_ADDR` で変えられる)。
- TUI はネットワークの待ち受けを持たず、API サーバを起動しない (FR-29)。
- API には利用者の認証が無い。
  このため、同じ端末で動く他のプロセスも API を呼べる。
- WebSocket の接続は `THIEF_WEB_ORIGINS` の Origin だけを受け付ける (FR-7)。
- AWS SSO のキャッシュ、Datadog のトークン、Google Cloud のプロジェクトのキャッシュは、権限 0600 のファイルに保存する。
- Secrets Manager と Parameter Store の値は一覧に載せず、backend でキャッシュしない (FR-5)。
- Datadog の OAuth のコールバックの応答は、クエリの値を反映しない固定の文言にする (反射型 XSS の防止)。
- HTTP の API は、`Origin` を WebSocket と同じ規則で `THIEF_WEB_ORIGINS` と照合し、許可した Origin にだけ CORS のヘッダを返す (`backend/internal/api/middleware.go` `corsMiddleware`、ADR 0032)。
  許可しない Origin からのリクエストのうち、メソッドが GET、HEAD、OPTIONS 以外のものは 403 `FORBIDDEN_ORIGIN` で拒否する。
- API サーバは `Host` を検査し、許可しないホストのリクエストを 421 `MISDIRECTED_REQUEST` で拒否する (DNS リバインディングの防止。`hostCheckMiddleware`、ADR 0032)。
- EC2 と ECS のセッションのルートは、Origin とアップグレードの検査を AWS のセッションの開始より前に行う (FR-7、issue 0217)。
- backend は 4xx と 5xx の応答の本文を、先頭 2048 バイトまでログに出す (`maxErrorBodyLogBytes`)。

### 運用

- ビルド、テスト、Lint、フォーマットは `mise run <task>` で実行する。
  `mise run check` が fmt、lint、vuln、test をまとめて実行する。
- pre-commit フック (`.pre-commit-config.yaml`、`prek` 経由) が `mise run fmt`、`mise run lint`、`mise run test` を実行する。
- CI は GitHub Actions に置き、`mise run check` を実行する。
  Linux では `go test -race` に cgo が要るため、CI では `CGO_ENABLED=1` でテストを実行する。
  ワークフローは未作成である (`.github/workflows/` が無い。「決定した論点」)。
- backend のログは `log/slog` の `slog.NewJSONHandler` で JSON として出す。
  現在の実装は既定のハンドラを使う (「規約と実装の差」)。
- 実 AWS アカウントなしの動作確認に `example/` (floci) を使う (`mise run example:up`、docker compose で floci のコンテナを起動する)。
- AWS と Google Cloud の公式アイコンは、ライセンスのためリポジトリに含めない。
  利用者が公式の配布物から展開する (`mise run frontend:fetch-aws-icons`、`mise run frontend:fetch-gcp-icons`)。

## 制約と前提

### 技術の制約

- backend は Go 1.26 系とする。
  `mise.toml` の Go と `backend/go.mod` の `toolchain` 行は同じバージョンに揃える (issue 0144)。
- `AGENTS.md` は backend のバイナリを `CGO_ENABLED=0` でビルドすることを基本とする。
  CGO を要するライブラリはこれに反する (issue 0196 では、この理由で Go の DuckDB を採らなかった)。
- 依存の追加は `AGENTS.md` の「依存関係の方針」の優先順に従う。
  標準ライブラリを第一とする。
- frontend は Vite、React 18、TypeScript (strict)、TanStack Query で作る。
  状態管理ライブラリとルーターは導入しない。
- TUI は `github.com/charmbracelet/bubbletea`、`bubbles`、`lipgloss` で作り、他の TUI フレームワークは導入しない (ADR 0033)。
  bubbletea は CLI の対話の選択 (`backend/internal/util/selecter.go` `Select`) で既に直接依存であり、`bubbles` と `lipgloss` を直接依存に加える。
- TUI は API サーバを経由せず、取得層をプロセス内で呼ぶ (ADR 0033)。
  サービスの追加は、`backend/internal/tui/service.go` の `Service` と `Row` を満たす型を置き、`backend/internal/cli/tui.go` の `Deps.Services` に足す形で行う (`AGENTS.md` の backend 「プロジェクト構造」)。
- backend の型と frontend の Raw 型は、ゴールデン JSON で突き合わせる (`backend/internal/contract/contract.go` `Registry`、`frontend/src/types/contract.check.ts`)。
- frontend は `http://localhost:8088`、backend は `127.0.0.1:8089` で動く。

### 外部仕様の前提

- AWS の認証情報は `~/.aws/` のファイルから読む。
  SSO のトークンのキャッシュは AWS CLI と互換にする。
- Google Cloud は ADC だけを使う。
  gcloud CLI には依存しない。
- CLI の `thief ec2 session` と `thief ecs exec` は、PATH にある `session-manager-plugin` を起動する (`backend/internal/cli/session_plugin.go`)。
  Web UI のターミナルと TUI の対話セッション (FR-30) は、このプラグインを使わず、`backend/internal/session` の DataChannel をプロセス内で扱う (FR-7、FR-30)。
- Cost Explorer と `DescribeRegions` の呼び出し先は us-east-1 とする。
- AWS の EC2 には、稼働台数の標準メトリクスが無い。
  Auto Scaling グループのメトリクスは有効化が任意である (issue 0186)。

### 規約と実装の差

`AGENTS.md` の記述と、2026-10-02 時点の実装は次の点で異なる。
どちらも実装を `AGENTS.md` に合わせる (「決定した論点」)。

| 項目 | `AGENTS.md` の記述 | 実装 |
| --- | --- | --- |
| HTTP サーバのタイムアウト | `ReadHeaderTimeout` と `IdleTimeout` を設定する。`ReadTimeout` と `WriteTimeout` は WebSocket の長時間接続と衝突するため設定しない | `ReadHeaderTimeout` だけを設定する (`backend/internal/api/server.go`) |
| ログのハンドラ | `slog.NewJSONHandler` を推奨する | 既定のハンドラを使う |

2026-09-26 時点で挙げた項目のうち、ミドルウェアとエラーの応答の形 (食い違い) と、翻訳リソースと Raw 型から Row 型への変換関数 (`AGENTS.md` の記述の不足) は、2026-10-02 に `AGENTS.md` を実装に合わせて直した。

## 未確定論点

| 論点 | 未確定の内容 | 解消の方法 |
| --- | --- | --- |
| Fargate Spot の料金 | データ源が無い (`docs/issues/pending/0059-feat-pricing-fargate-spot-rates.md`) | 利用できるデータ源が見つかった時点で、開発者が対応を決める |
| TUI の第 2 段階以降の範囲 | 第 2 段階以降で扱うと決めた Google Cloud、Datadog、TiDB Cloud のビュー、分析のビュー、リソースの変更操作の、段階ごとの範囲と順序。材料に無いコストと料金のビュー、オブジェクトのプレビューと SQL 検索、分割表示、TUI の状態の永続化 (起草者の整理) を TUI で扱うか | 第 1 段階 (FR-29、FR-30) の完了後に開発者が決める |
| TUI の Drawer のサービス固有のタブ | Web UI の Drawer にあるサービス固有のタブ (ECS の Services、Tasks、RDS の Instance Parameters、Cluster Parameters など) を TUI に置くか | サービスごとの要望が出た時点で、開発者がサービス単位で決める |
| FR-29 と FR-30 の優先度 | 起草者が付けた Must で確定するか | 開発者が確認する |
| TUI の中の SSO の再ログイン | FR-9 のデバイス認可を TUI の中で行うか、`thief sso login` の案内に留めるか | 第 1 段階の完了後に開発者が決める |

## 決定した論点

次の論点は、2026-10-02 に開発者が決めた。
実装が要るものは、後続の作業として `docs/issues/TODO.md` に追加した。

| 論点 | 決定 | 後続の作業 |
| --- | --- | --- |
| CORS の方針 | HTTP の API も `THIEF_WEB_ORIGINS` の Origin だけを許可し、`Host` を検査する (ADR 0032) | 無い (issue 0200 と issue 0217 で修正済み) |
| 機能要求の優先度 | 起草者が付けた優先度で確定する | 無い |
| 利用状況の成功指標 | 置かない。thief は利用状況を記録しない | 無い |
| 性能の目標値 | 置かない。遅さを計測で確かめた時点で issue を起票する | 無い |
| `thief athena query` の行数 | CLI は全ページを取得し、上限を設けない。Web の画面は現在の「さらに読み込む」で足りる (FR-13) | 無い |
| Google Cloud のサイドバーのバッジ | AWS 側と同じ `useCachedQueryData` に揃える (FR-2) | `GcpSidebar.tsx` の書き換え |
| Cost Explorer の予測の Web 表示 | Web の画面に予測を表示する (FR-11) | 予測の表示の追加 |
| CloudFront の invalidation | Drawer に作成の UI を足す (FR-3) | invalidation の UI の追加 |
| Datadog の静的キーの名前 | `CHANGES.md` の `DD_API_KEY` と `DD_APP_KEY` を、実装の `DATADOG_API_KEY` と `DATADOG_APP_KEY` に直す | 無い (2026-10-02 に修正済み) |
| Datadog のスコープ | `usage_read` のままとする (FR-21) | 無い |
| CI | GitHub Actions で `mise run check` を実行する | ワークフローの追加 |
| HTTP サーバのタイムアウト | `IdleTimeout` を足す。`AGENTS.md` は、`ReadTimeout` と `WriteTimeout` を設定しない記述に直す | `IdleTimeout` の追加 |
| ミドルウェア | `AGENTS.md` を実装 (Host の検査、CORS、アクセスログ) に合わせる | 無い (2026-10-02 に修正済み) |
| ログのハンドラ | `slog.NewJSONHandler` で JSON にする | JSON のハンドラへの切り替え |
| エラーの応答の形 | `AGENTS.md` を ADR 0009 の平らな形に合わせる | 無い (2026-10-02 に修正済み) |
| `AGENTS.md` の記述の不足 | 翻訳リソースの `en` と、変換関数の `normalizeGcp.ts`、`normalizePricing.ts`、`normalizeQuery.ts` を `AGENTS.md` に書き足す | 無い (2026-10-02 に修正済み) |
| SQL 検査の誤検知 | コメントと文字列リテラルを除いてから検査する (FR-16) | `sqlguard` の書き換え |
| `thief bq query` の検査の例外 | 検査を掛け、書き込みを許すフラグは設けない (FR-16) | `thief bq query` への検査の適用 |

次の論点は、2026-10-23 に開発者が決めた。

| 論点 | 決定 | 後続の作業 |
| --- | --- | --- |
| TUI の追加 | Web UI に相当する操作を端末の中で行う全画面の TUI (`thief tui`) を追加する。第 1 段階は AWS の一覧、Drawer (Overview と Tags)、対話セッションとする (「TUI の第 1 段階の範囲」)。「やらないこと」の「ブラウザ以外の GUI のクライアント」から TUI を除く | FR-29 と FR-30 の実装 |
| TUI の接続方式 | API サーバを経由せず、取得層をプロセス内で直接呼ぶ (ADR 0033) | 無い |
| TUI のフレームワーク | bubbletea、bubbles、lipgloss を使う。tview は採らない (ADR 0033) | 無い |
| TUI の第 1 段階の範囲 | AWS の 19 サービスの一覧、Drawer (Overview と Tags)、EC2 と ECS への対話セッション。Google Cloud、Datadog、TiDB Cloud のビュー、分析のビュー、リソースの変更操作は第 2 段階以降で扱い、段階ごとの範囲と順序は第 1 段階の完了後に決める (「未確定論点」)。コストと料金のビュー、オブジェクトのプレビューと SQL 検索、分割表示、TUI の状態の永続化は材料に無く、第 1 段階では扱わない (「スコープ」) | `ec2` 以外の 18 サービスの追加、ECS Exec の追加 |
| TUI の進め方 | `ec2` だけのプロトタイプを先に作って体験を確かめ、その後に PRD の改訂、ADR 0033、issue の起票、実装の順で正規の手順に戻す | PRD の改訂 (この改訂)、ADR 0033、`AGENTS.md`、`CHANGES.md`、`docs/issues/TODO.md` の TUI の記述は書き終えており、プロトタイプを issue として検証したうえで同じコミットで入れる。その後に残りの issue を実装する (プロトタイプの動作は 2026-10-23 に確認済み) |

次の論点は、2026-10-26 に issue 0224 の実装で、issue の設計判断の第一候補のとおりに決めた。開発者の確認は実装の報告で得る。

| 論点 | 決定 | 後続の作業 |
| --- | --- | --- |
| TUI の ECS のタスクとコンテナの選び方 | 選択肢で選ぶ。クラスタ行で `s` を押すと、実行中のタスク、次に ECS Exec が有効なコンテナを、サービスの切替と同じ絞り込み付きの選択肢で順に選ぶ。候補が 1 件なら自動で選ぶ (FR-30)。Drawer のタブで選ぶ案は、TUI の Drawer を Overview と Tags に固定しており、Web UI でも接続先の選択がタブではなく Terminal タブの中の選択であるため採らない | 開発者がこの方式を確認する (確認で方式が変わればこの行と FR-30 を直す)。FR-30 の ECS の受け入れ基準を実 AWS で確認する (issue 0224 では未確認) |

## 関連資料

- 設計判断の記録: `docs/adr/`
- TUI の接続方式とフレームワーク: ADR 0033 (`docs/adr/0033-tui-in-process-bubbletea.md`)
- TUI のフロート端末: ADR 0034 (`docs/adr/0034-tui-floating-terminal-window.md`)
- issue: `docs/issues/closed/`、`docs/issues/pending/`、`docs/issues/TODO.md`
- CORS の不具合と修正の方針: `docs/issues/closed/0200-bug-cors-reflects-any-origin.md`、ADR 0032 (`docs/adr/0032-api-origin-and-host-checks.md`)
- 変更履歴: `CHANGES.md`
- リポジトリ規約: `AGENTS.md`
- セットアップと起動: `README.md`
- ローカルの動作確認: `example/README.md`
- AWS サービスの追加手順: `.claude/skills/add-aws-service/SKILL.md`
- RFC 8628 (OAuth 2.0 Device Authorization Grant): https://www.rfc-editor.org/rfc/rfc8628
- RFC 6749 (OAuth 2.0): https://www.rfc-editor.org/rfc/rfc6749
- RFC 7636 (PKCE): https://www.rfc-editor.org/rfc/rfc7636
- RFC 7591 (OAuth 2.0 Dynamic Client Registration): https://www.rfc-editor.org/rfc/rfc7591
- RFC 2617 (HTTP Digest Authentication): https://www.rfc-editor.org/rfc/rfc2617
