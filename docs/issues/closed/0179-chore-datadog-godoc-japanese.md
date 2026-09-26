# internal/datadog パッケージの exported 識別子の godoc を日本語に統一する

Created: 2026-09-16
Model: Claude Opus 5
Completed: 2026-09-17

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] internal/datadog パッケージの exported 識別子の godoc コメントを日本語化したい
>     - issue 0169 のレビューで判明。issue 0164〜0168 で追加された既存の exported 識別子 (ListOrgs, OrgInfo 等) が英語の godoc のままで、グローバル規約の日本語コメント原則に反している
>     - issue 0169 自身が追加した新規識別子だけを日本語化すると同一パッケージ内で英語/日本語が混在し不統一になるため、パッケージ全体をまとめて対応する

グローバル規約 `~/.codex/AGENTS.md` の「原則」は「コメントは全て日本語」と定め、`AGENTS.md` の「backend コーディングスタイル」は exported な識別子に `// Foo は ...` の形式で godoc を付けることを定めている。

`backend/internal/datadog` の非テストファイルは `client.go`、`dashboards.go`、`errors.go`、`metrics.go`、`organizations.go`、`usage.go` である。
このパッケージにメソッド (レシーバ付き関数) は無く、exported 識別子は型、関数、定数の合計 32 個である。すべてに godoc は付いているが、言語が揃っていない。

| 言語 | 件数 |
| --- | --- |
| 日本語のみ | 12 |
| 英語のみ | 18 |
| 1 文目が英語で続きが日本語 | 2 |
| godoc 無し | 0 |

英語のままの例は `client.go` の `NewConfiguration` (「creates a Datadog API configuration for the given site.」)、`client.go` の `UsageMeteringV2API` (「wraps the Datadog v2 usage metering API.」)、`errors.go` の `IsForbidden`、`usage.go` の `CostInfo` (「represents a Datadog cost line item.」)、`dashboards.go` の `ListDashboards` と `GetDashboard` である。
日本語の例は `organizations.go` の `OrgInfo` (「現在の認証情報でアクセスできる Datadog の組織を表す。」) と `ListOrgs`、`dashboards.go` の `WidgetKind` と `DashboardInfo`、`metrics.go` の `MetricPoint` である。
英日混在は `client.go` の `OrganizationsV2API` と `metrics.go` の `QueryMetrics` の 2 件で、いずれも 1 文目が英語、続く補足段落が日本語である。

ファイル単位では `client.go` と `usage.go` がほぼ英語、`organizations.go` が全て日本語、`dashboards.go` と `metrics.go` が日本語主体で 1 件から 2 件だけ英語または混在という状態にある。
同じパッケージの同じ層のコードで言語が割れており、読む側がどちらの言語で書かれているかを都度判断することになる。

非 exported 識別子の godoc と行内コメントは、`managedOrgsView`、`orgRef`、`managedOrgsViewOf`、`warnSDKRejections`、`redecode`、`extractWidgets`、`widgetInfo`、`seriesName`、`costInfosFromResponse` などを含めてすべて日本語である。
テストファイル 5 本のコメントも、`usage_test.go` のヘルパー `newTestV2API` の godoc 1 行を除いてすべて日本語である。
つまり英語が残っているのは exported 識別子の godoc に偏っている。

他のパッケージでは `backend/internal/api` と `backend/internal/gcp` が日本語の godoc でほぼ統一されている。
`backend/internal/aws` には英語の書き出しを含むファイルが 31 本残っているが、こちらは複数の issue にまたがって長期に追加されたもので、本 issue の対象ではない (理由は「対応方針」に記す)。

## 対応方針

`backend/internal/datadog` の非テストファイル 6 本にある exported 識別子の godoc を日本語に統一する。
挙動は変えない。コメントの文言だけを変更する。

文体は `AGENTS.md` の「exported な識別子には godoc コメントを付け、`// Foo は ...` の形式で名前から書き出す」に従う。
既存の日本語 godoc と同じ形の例は次のとおりで、これをテンプレートにする。

- `backend/internal/aws/s3_object.go` の `// S3ObjectResource は S3 バケット内の 1 オブジェクトを表す。`
- `backend/internal/aws/s3_object.go` の `// ListS3Objects は指定バケット (と prefix) のオブジェクト一覧を返す。`
- `backend/internal/api/datadog_auth_context.go` の `// ErrDatadogNoCredentials は Datadog を呼ぶ資格情報が 1 つも使えないことを表す。`

英日混在の 2 件 (`OrganizationsV2API` と `QueryMetrics`) は、英語の 1 文目だけを日本語に置き換える。後続の日本語の補足段落はそのまま残す。

`usage.go` の `CostInfo` はフィールドごとの godoc を持たない。本 issue はフィールドの godoc を新規に追加しない (「扱わない範囲」に記す)。

### 採らなかった案

- **`internal/aws` を含む backend 全体をまとめて日本語化する**。統一の範囲としては最も筋が通るが、`internal/aws` だけで英語の書き出しを含むファイルが 31 本あり、1 つの issue で実装、レビュー、close できる大きさを超える。また `internal/aws` は複数の issue にまたがって追加されたもので、TODO の出所 (issue 0169 のレビュー) とも対象が異なる。`internal/datadog` に限れば、issue 0164 から 0169 までの一連の追加で生まれた不統一を、同じ範囲でまとめて解消できる。
- **`internal/datadog` を英語に統一する**。パッケージ内の統一という目的だけなら成立するが、グローバル規約「コメントは全て日本語」に反する。また日本語の非 exported コメントと行内コメントも書き換えることになり、変更範囲が広がる。
- **godoc の言語を lint で強制する仕組みを本 issue で入れる**。現状の `mise run backend:lint` は `go vet`、`staticcheck`、`govulncheck` の 3 つで、`.golangci.yml` も revive の設定も存在せず、コメントの言語を検査する仕組みは無い。言語を機械的に判定する lint は誤検知 (固有名詞や API 名のみの行) を避けにくく、どのルールをどう設定するかという別の判断が要る。本 issue はコメントの書き換えに限定し、再発防止の仕組みは扱わない。
- **パッケージレベルの doc comment (`doc.go`) を新設する**。`backend/internal/datadog` にはパッケージレベルの godoc が存在しない。TODO は既存の exported 識別子の日本語化を求めており、新規の doc の追加は求めていない。範囲を広げない。

## 完了条件

- `backend/internal/datadog` の非テストファイル (`client.go`、`dashboards.go`、`errors.go`、`metrics.go`、`organizations.go`、`usage.go`) にある exported 識別子 32 個すべての godoc が日本語で書かれている。英語の文が 1 つも残っていない。
- 各 godoc が `// <識別子名> は` で始まる。
- 英日混在だった `client.go` の `OrganizationsV2API` と `metrics.go` の `QueryMetrics` について、英語の 1 文目が日本語に置き換わり、既存の日本語の補足段落が残っている。
- `usage_test.go` のヘルパー `newTestV2API` の godoc が日本語になっている。この識別子は非 exported であり TODO が挙げる「exported 識別子」の範囲外だが、パッケージ内に残る唯一の英語コメントであり、1 行の変更で「パッケージ全体をまとめて対応する」という TODO の意図を満たせるため対象に含める。
- コメント以外の変更が無い。`git diff` の変更行が、コメント行とコメント行の追加削除だけで構成されている。
- `mise run check` が通る。

### 扱わない範囲

- `backend/internal/aws` をはじめとする他パッケージの godoc は変更しない。
- `usage.go` の `CostInfo` のフィールド (`Month`、`AccountName`、`OrgName`、`ProductName`、`ChargeType`、`Cost`) に godoc を新規追加しない。既にフィールドごとの godoc を持つ型 (`DashboardInfo`、`WidgetInfo`、`MetricPoint` など) のフィールドコメントは日本語であり、変更しない。
- パッケージレベルの doc comment (`doc.go`) を追加しない。
- コメントの言語を検査する lint を追加しない。

## 関連

- `docs/issues/closed/0169-feat-datadog-dashboards.md` が本 TODO の出所である。同 issue は新規識別子だけを日本語化するとパッケージ内で不統一になることを理由に、対応を TODO.md へ送った。

## 解決方法

`backend/internal/datadog` の非テストファイル 6 本 (`client.go`、`dashboards.go`、`errors.go`、`metrics.go`、`organizations.go`、`usage.go`) にある exported 識別子の godoc を日本語に統一した。挙動を変える変更は行っていない。

- 英語のみだった 18 個と、英日混在だった 2 個 (`client.go` の `OrganizationsV2API`、`metrics.go` の `QueryMetrics`) を対応方針のテンプレートに従って日本語化した。混在の 2 件は英語の 1 文目だけを日本語に置き換え、既存の日本語の補足段落はそのまま残した。`organizations.go` は起票時点で既に全て日本語だったため変更していない。
- `usage_test.go` の非 exported ヘルパー `newTestV2API` の godoc も、完了条件のとおりパッケージ内に残る唯一の英語コメントとして合わせて日本語化した。
- `usage.go` の `CostInfo` にはフィールドごとの godoc を新規追加していない (扱わない範囲どおり)。
- 多観点レビューの観点 2 (回帰と整合) で、`GetEstimatedCost` の翻訳が英語原文の「当月または前月、またはその両方」というニュアンス (`endMonth` が空文字なら前月を省略できる、`params.EndMonth` の代入が条件付きであることに対応) を落とし「常に両方を返す」という誤った含意になっていた指摘を受けた。コード (`if endMonth != "" { params.EndMonth = &end }`) を確認したうえで「当月または前月、あるいはその両方の見積もりコストを返す」に修正し、3 観点の追加レビュー (1 ラウンド) で指摘ゼロを確認した。
- `git diff` の変更行はコメント行の追加削除のみで構成されている。`mise run check` が通ることを確認した。
