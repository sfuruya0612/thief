# thief PRD

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Draft

## 概要

thief は、AWS、Google Cloud、Datadog、TiDB Cloud のリソースとコストを、手元の端末から 1 つの Web UI と CLI で閲覧する開発者向けツールである。
1 つの Go バイナリ (`thief`) が API サーバと CLI を兼ね、ブラウザで動く React の SPA がその API を使う。
## 背景と課題

### 本 PRD を書く理由

thief の要求はこれまで `docs/issues/TODO.md` の箇条書きと個々の issue にしか書かれていない。
そのため、製品全体として何を満たすべきかを 1 か所で示す文書が無い。
本 PRD は、実装済みのコード、`docs/issues/` の issue 199 件 (closed 198 件と pending 1 件)、`docs/issues/TODO.md` から要求を逆算して書き起こしたものである。
  本 PRD の作成中に起票した issue 0200 (CORS の不具合) は材料に含めない。
以下では、これら 3 つを**材料**と呼ぶ。

- issue は 2026-07-12 から 2026-09-26 まで git 管理の対象外だった (コミット b4ca037、ADR 0027)。
  2026-09-26 に `docs/issues/` へ移し、git で管理することにした (ADR 0028)。
  その間に書いた issue の書き換えの経過は、リポジトリの履歴に残っていない。
- 設計判断の根拠は issue の本文とコミットメッセージに散っている。
  本 PRD は要求だけを扱い、判断の記録は `docs/adr/` に分けて置く。
- リポジトリ規約 `AGENTS.md` の記述と実装が食い違う箇所がある (「制約と前提」の「規約と実装の差」を参照)。
  規約だけを読んで実装の振る舞いを知ることはできない。

### thief が解く課題

材料に課題として書かれているものと、材料には課題として書かれておらず実装から推定したものを分けて示す。

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

## 目的

- 利用者が 1 つの UI から、4 つの外部サービス (AWS、Google Cloud、Datadog、TiDB Cloud) のリソース一覧、詳細、コストに到達できる。
- 利用者がブラウザだけで EC2 と ECS のシェルに入れる。
- 利用者が SSO トークンの期限切れを画面上で知り、画面を離れずに再ログインできる。
- 利用者が CLI からも Web UI と同じ主要なリソースを一覧でき、結果をタブ区切りか CSV で受け取れる。
- 利用者の認証情報と、外部サービスから取得したデータを、利用者の端末の外に送らない。
  これらの送り先は各外部サービスの公式 API だけである。

## 成功指標

材料には利用状況の計測の記録が無い。
そのため、本 PRD の成功指標はリポジトリ上で機械的に測れるものに限る。
利用状況の指標は「未確定論点」に残す。

| 指標 | 目標 | 測定方法 |
| --- | --- | --- |
| 品質ゲートの通過 | fmt、lint、vuln、test がすべて成功する | リポジトリのルートで `mise run check` を実行し、終了コードが 0 であることを確認する |
| backend と frontend の型契約の一致 | 不一致 0 件 | `frontend/` で `npm run lint` を実行する。`frontend/src/types/contract.check.ts` の型検査が失敗しないことを確認する |
| AWS サービスの網羅 | `frontend/src/lib/serviceMeta.ts` の `SERVICES` にある 23 サービスすべてに、一覧の表示先がある | `SERVICES` の各 key が、`SERVICE_TO_PATH` の対応先か、専用ビュー (athena、cloudwatchlogs、costexplorer、pricing) のどちらかを持つことをコードで確認する |
| CLI の一覧コマンドの存在 | FR-10 の表の一覧コマンドが 27 個すべて存在する。Web にだけある機能 (Pricing、分割表示など) は対象外とする | 表の各コマンドに `--help` を付けて実行し、終了コードが 0 で、出力の `Usage:` の次の行が、前後の空白を除いたとき表のコマンドで始まることを確認する |

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

「機能要求」に挙げた機能は、すべて対象とする。

### やらないこと

- 外部サービスのリソースの作成、削除、設定変更 (FR-5、FR-6、FR-20 に挙げた値とオブジェクトの書き込みを除く)。
  CloudFront の invalidation を作る API (`POST /api/aws/profiles/{profile}/cloudfront/{id}/invalidations`) は実装済みだが画面が無く、扱いは「未確定論点」で決める。
  利用者の端末の中のファイルの作成と削除 (FR-18 のスニペット、`thief sso generate-config` による `~/.aws/config` の生成) は、この対象外に含めない。
- 書き込みの SQL の実行 (Athena と BigQuery は読み取りだけを通す。例外は CLI の `thief bq query` で、FR-16 に書く)。
- 複数人で共有するサーバとしての運用 (利用者の認証、認可、監査ログを持たない)。
- thief 本体の Docker による起動 (2026-07-18 のコミット cb2da77 で削除した)。
  `example/` のエミュレータ (floci) は docker compose で起動し、これは対象外に含めない。
- ブラウザ以外の GUI のクライアント (CLI は除く。Flutter 時代の macOS デスクトップ対応は廃止した。`AGENTS.md` の frontend 「基本方針」)。
- URL によるルーティング (react-router などを導入しない。`AGENTS.md` の frontend 「基本方針」)。
- EC2 の台数の時系列グラフ (標準で取れるメトリクスが無いため撤回した。issue 0186)。
- Fargate Spot の料金表示 (データ源が無いため保留した。`docs/issues/pending/0059-feat-pricing-fargate-spot-rates.md`)。
- エミュレータ (`example/` の floci) で再現できない機能の、エミュレータ上での動作確認 (SSO ログイン、ターミナル、Cost Explorer と請求系)。

## 機能要求

優先度は材料に記録が無い。
本 PRD では、起草者が次の基準で付けた。

- **Must**：欠けると UC の主目的が果たせない機能
- **Should**：UC の主目的は果たせるが、手間が増える機能
- **Could**：補助的な機能と、`docs/issues/TODO.md` で未着手の機能

この付け方は「未確定論点」で確認を求める。

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

受け入れ基準:

- サイドバーに表示されるサービスの数が 23 である。
- 一覧を開いていないサービスのバッジは、件数を表示しない。
- ECS の一覧を開くと、クラスタごとの稼働タスク数のグラフ (Tasks per cluster) が表示される。

**FR-3 リソースの詳細 (Drawer)** (Must)

- 一覧で行を選ぶと Drawer を開き、Overview と、サービスごとのタブを表示する (`frontend/src/components/Drawer/Drawer.tsx` `DRAWER_TABS`)。
- タブの内容はタブを開いたときに取得する。
- タブの取得が失敗したとき、AWS が返したエラーの本文をそのまま (英語で) 表示する (issue 0075、0082、`frontend/src/components/Drawer/drawerError.tsx`)。

受け入れ基準:

- ECS の Drawer に Overview、Services、Tasks、Instances、Terminal、Tags のタブがある。
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

受け入れ基準:

- ターミナルを閉じた後、`aws ssm describe-sessions --state Active` の結果にそのセッションが残らない。
- `THIEF_WEB_ORIGINS` に含まれない Origin からの WebSocket 接続は拒否される。

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
- SIGINT か SIGTERM を受けたら処理を取り消し、終了コード 130 で終わる (`backend/internal/cli/run.go` `interruptExitCode`)。
- 設定は、コマンドライン引数、環境変数、設定ファイル、既定値の順で優先する。

受け入れ基準:

- `thief ec2 ls -o csv` の出力がカンマ区切りで、1 行目がヘッダである。
- `thief ec2 ls -o csv --no-header` の出力にヘッダ行が無い。
- 一覧の実行中に Ctrl-C を押すと、終了コードが 130 になる。
- 環境変数 `THIEF_REGION` と `-r` の両方を指定すると、`-r` の値が使われる。

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
- コストの予測は CLI (`thief cost forecast`) で表示する。

受け入れ基準:

- 10 サービス分のコストがある期間を開くと、グラフの系列がちょうど 9 本 (上位 8 本と Other) である。
- CSV 出力の行数が、画面の集計表の行数と一致する。

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
  CLI の `thief athena query` は全ページを取得する。
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

- Athena と BigQuery のクエリは、INSERT、UPDATE、DELETE、MERGE、CREATE、DROP、ALTER、TRUNCATE、GRANT、REVOKE、UNLOAD のいずれかの語を含むとき拒否する (`backend/internal/sqlguard/sqlguard.go`)。
- この検査は、API サーバを通るすべてのクエリと、CLI の `thief athena query` に掛ける。
  CLI の `thief bq query` は例外で、検査を通さずに任意の SQL を実行する (`backend/internal/bigquery/query.go` `ExecuteQueryUnrestricted`、以前の CLI との互換のため)。
- この検査は語の一致で判定する。
  コメントや文字列リテラルの中の語でも拒否する。

受け入れ基準:

- API の Athena と BigQuery のクエリの実行と `thief athena query` で、上記 11 語のそれぞれを含む SQL が、大文字小文字を問わず拒否される (`thief bq query` は対象外)。
- `SELECT 'drop' AS x` が拒否される (現行の判定の仕様として)。

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
- 外部 API の並列呼び出しは、同時実行数を上限付きにする。
  上限は既定 30、WAF と IAM は 10 とする。
- Cloud Run の一覧は、ロケーションごとのジョブの取得を並列にする。
  43 ロケーションで、一覧全体が 44.9 秒から 6.3 秒に (ロケーションごとのジョブの取得は 43.4 秒から 4.8 秒に) 短くなった実測がある (issue 0043)。
- 大きな結果は件数の上限で打ち切る: オブジェクトの一覧 1000 件、オブジェクトの SQL 検索 10000 行。
- 性能の目標値 (応答時間の上限など) は材料に無い。
  「未確定論点」に残す。

### 可用性

- thief は利用者の端末で動く単一のプロセスである。
  冗長化と常時稼働は対象外とする。
- 外部 API の一部の取得失敗で、一覧全体を失敗にしない (FR-4)。
- backend の停止中も frontend は接続待ちを表示する (FR-27)。
- backend は SIGINT と SIGTERM で待ち受けを止め、処理中のリクエストの完了を最大 5 秒待つ (`backend/internal/cli/server.go` `serverShutdownTimeout`)。

### セキュリティ

- API サーバは既定で `127.0.0.1:8089` で待ち受ける (`THIEF_LISTEN_ADDR` で変えられる)。
- API には利用者の認証が無い。
  このため、同じ端末で動く他のプロセスも API を呼べる。
- WebSocket の接続は `THIEF_WEB_ORIGINS` の Origin だけを受け付ける (FR-7)。
- AWS SSO のキャッシュ、Datadog のトークン、Google Cloud のプロジェクトのキャッシュは、権限 0600 のファイルに保存する。
- Secrets Manager と Parameter Store の値は一覧に載せず、backend でキャッシュしない (FR-5)。
- Datadog の OAuth のコールバックの応答は、クエリの値を反映しない固定の文言にする (反射型 XSS の防止)。
- HTTP の API の CORS は、リクエストの `Origin` をそのまま `Access-Control-Allow-Origin` に返す (`backend/internal/api/middleware.go` `corsMiddleware`)。
  そのため、利用者のブラウザで開いた任意のサイトが、API の応答 (シークレットの値を含む) を読み、書き込みの API を呼べる。
  この現状の課題の扱いは「未確定論点」に残す。
- backend は 4xx と 5xx の応答の本文を、先頭 2048 バイトまでログに出す (`maxErrorBodyLogBytes`)。

### 運用

- ビルド、テスト、Lint、フォーマットは `mise run <task>` で実行する。
  `mise run check` が fmt、lint、vuln、test をまとめて実行する。
- pre-commit フック (`.pre-commit-config.yaml`、`prek` 経由) が `mise run fmt`、`mise run lint`、`mise run test` を実行する。
- CI は無い (`.github/workflows/` にワークフローが無い)。
- backend のログは `log/slog` で出す。
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
- backend の型と frontend の Raw 型は、ゴールデン JSON で突き合わせる (`backend/internal/contract/contract.go` `Registry`、`frontend/src/types/contract.check.ts`)。
- frontend は `http://localhost:8088`、backend は `127.0.0.1:8089` で動く。

### 外部仕様の前提

- AWS の認証情報は `~/.aws/` のファイルから読む。
  SSO のトークンのキャッシュは AWS CLI と互換にする。
- Google Cloud は ADC だけを使う。
  gcloud CLI には依存しない。
- CLI の `thief ec2 session` と `thief ecs exec` は、PATH にある `session-manager-plugin` を起動する (`backend/internal/cli/session_plugin.go`)。
  Web UI のターミナルは、このプラグインを使わない (FR-7)。
- Cost Explorer と `DescribeRegions` の呼び出し先は us-east-1 とする。
- AWS の EC2 には、稼働台数の標準メトリクスが無い。
  Auto Scaling グループのメトリクスは有効化が任意である (issue 0186)。

### 規約と実装の差

`AGENTS.md` の記述と、2026-09-26 時点の実装は次の点で異なる。
翻訳リソースと変換関数の行は食い違いではなく、`AGENTS.md` の記述が足りない点である。

| 項目 | `AGENTS.md` の記述 | 実装 |
| --- | --- | --- |
| HTTP サーバのタイムアウト | `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout` を必ず設定する | `ReadHeaderTimeout` だけを設定する。WebSocket の長時間接続と衝突するため (`backend/internal/api/server.go` のコメント) |
| ミドルウェア | パニックの回復、リクエスト ID、アクセスログ、タイムアウト、リクエストサイズの制限を入れる | CORS とアクセスログだけ (`backend/internal/api/middleware.go`) |
| ログのハンドラ | `slog.NewJSONHandler` を推奨する | 既定のハンドラを使う |
| エラーの応答の形 | backend の節は `{"error": {"code": "...", "message": "..."}}` 等と書く | `{"error": "...", "code": "...", "details": ...}` の平らな形で、`message` は無い (`backend/internal/api/models.go` `ErrorResponse`、ADR 0009) |
| 翻訳リソース | `src/i18n/locales/ja/` に 14 ネームスペースがあると書き、`en` には触れていない (差ではなく記述の不足) | `ja` と `en` にそれぞれ 14 ネームスペースがある (`frontend/src/i18n/locales/`) |
| Raw 型から Row 型への変換関数 | `lib/normalize.ts` と `lib/normalizeNonAws.ts` に集約すると書く (差ではなく記述の不足) | `normalizeGcp.ts`、`normalizePricing.ts`、`normalizeQuery.ts` にも分かれる (ADR 0008) |

## 未確定論点

| 論点 | 未確定の内容 | 解消の方法 |
| --- | --- | --- |
| CORS の方針 | HTTP の API が任意の Origin を許可している。`THIEF_WEB_ORIGINS` に揃えるか、別の防御 (トークンなど) を入れるかが決まっていない | 開発者が方針を決め、ADR に残す。不具合として issue 0200 (`docs/issues/0200-bug-cors-reflects-any-origin.md`) に起票済みで、現在の判断は ADR 0010 (`docs/adr/0010-local-api-without-authentication.md`) にある。提案: HTTP の API も `THIEF_WEB_ORIGINS` の Origin だけを許可する |
| 機能要求の優先度 | 優先度は起草者が付けたもので、材料に根拠が無い | プロダクトの責任者が FR ごとの優先度を確認する |
| 利用状況の成功指標 | 利用頻度や、コンソールを開く回数の削減などの指標が無い。thief は利用状況を記録しない | プロダクトの責任者が、計測するか、指標を置かないかを決める |
| 性能の目標値 | 一覧の応答時間などの上限が決まっていない | 開発者が、測る対象と上限を決める |
| `thief athena query` の行数 | CLI は結果の全ページを取得し、行数の上限が無い (`docs/issues/TODO.md`) | 開発者が上限の既定値と指定方法を決める。提案: 上限を指定するフラグを足す |
| Google Cloud のサイドバーのバッジ | `frontend/src/views/GcpSidebar.tsx` は `useCachedQueryData` を使っていない。TanStack Query の実装の詳細に依存している (`docs/issues/TODO.md`) | 開発者が AWS 側と揃えるかを決める |
| Fargate Spot の料金 | データ源が無い (`docs/issues/pending/0059-feat-pricing-fargate-spot-rates.md`) | 利用できるデータ源が見つかった時点で、開発者が対応を決める |
| Cost Explorer の予測の Web 表示 | `frontend/src/api/queries.ts` に `useCostForecast` があるが、画面から使う箇所が無い。予測は CLI だけで表示できる | 開発者が、Web に予測を表示するか、フックを削除するかを決める |
| CloudFront の invalidation | backend の `POST /api/aws/profiles/{profile}/cloudfront/{id}/invalidations` はあるが、frontend から呼ぶ箇所が無い | 開発者が、UI を足すか、エンドポイントを削除するかを決める |
| Datadog の静的キーの名前 | 実装は `DATADOG_API_KEY` と `DATADOG_APP_KEY` を読む。`CHANGES.md` は `DD_API_KEY` と `DD_APP_KEY` と書いている | 開発者が `CHANGES.md` の記述を実装に合わせる |
| Datadog のスコープ | `usage_read` だけでダッシュボードとメトリクスの API が呼べるかを、材料では確認していない | 開発者が実際の組織で確認し、不足ならスコープを足す |
| CI | CI が無く、品質ゲートは各開発者の pre-commit フックに依存する | 開発者が CI を置くかを決める |
| `AGENTS.md` と実装の差 | 「規約と実装の差」の 4 項目 | 開発者が、規約を実装に合わせるか、実装を規約に合わせるかを項目ごとに決める |
| SQL 検査の誤検知 | コメントや文字列リテラルの中の語でも拒否する (FR-16) | 開発者が、誤検知を許容するか、SQL を構文解析するかを決める |
| `thief bq query` の検査の例外 | CLI の `thief bq query` だけが読み取り専用の検査を通らず、書き込みの SQL を実行できる (FR-16)。以前の CLI との互換を保つ理由が今もあるかが確認されていない | 開発者が、例外を残すか、検査を掛けるかを決める。提案: 検査を掛け、書き込みが要るときは明示のフラグを求める |

## 関連資料

- 設計判断の記録: `docs/adr/`
- issue: `docs/issues/closed/`、`docs/issues/pending/`、`docs/issues/TODO.md`
- CORS の不具合: `docs/issues/0200-bug-cors-reflects-any-origin.md`
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
