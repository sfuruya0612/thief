# AWS リソース一覧の関連取得系フィールドで、取得失敗による 0 と真の 0 を UI 上で区別できるようにする

Created: 2026-07-27
Model: Claude Sonnet 5
Completed: 2026-08-07

## 背景

docs/issues/closed/0088 (WAF の Associated 列が 0 になる不具合の修正) の完了条件で、この課題を別 issue として起票することが定められていた。

`handleIgnoredErr` (`backend/internal/aws/errors.go:84-93`) は、権限不足やスロットリング等で取得に失敗した詳細値を slog.Warn に記録した上で 0 件 (または空) 扱いに縮退させる。この方針自体は docs/issues/closed/0067 と docs/issues/closed/0075 で確立されたもので、一覧取得全体を失敗させずに劣化させる設計として妥当だが、結果として「本当に関連リソースが 0 件」と「取得に失敗して 0 として表示されている」を frontend 側で区別できない。

WAF の `associated_count` はその一例であり、他にも同じ縮退方針を使うフィールド (一覧取得に付随する関連カウント・集計値全般) で同様の曖昧さが生じ得る。

## 症状

`handleIgnoredErr` 経由で 0 に縮退したフィールドは、真の 0 と見た目上区別がつかない。ユーザーは表示された 0 が「関連が無い」のか「取得できなかった」のかを、backend の Warn ログを直接確認しない限り判別できない。

## 影響範囲の調査が必要な事項

- `handleIgnoredErr` を経由して 0 / 空に縮退させている箇所の棚卸し (WAF の `associated_count` 以外にどのフィールドが該当するか)。
- 縮退の事実を API レスポンスにどう含めるか (例: フィールドごとの `*_fetch_failed` フラグ、あるいは docs/issues/closed/0075 が Drawer サブタブに導入した DrawerError 方式の一覧行版)。
- frontend でどう表示するか (値のグレーアウト、警告アイコン、ツールチップ等)。

## 検討方針 (未確定)

具体的な設計判断 (API レスポンス形状の変更、frontend の表示方式) は本 issue 内で改めて検討する。docs/issues/closed/0075 の Drawer サブタブにおける「エラーは空表示ではなく明示する」という表示規則を、一覧行の集計フィールドにも一貫して適用できるかどうかがこの issue の中心的な論点になる。

## 調査結果 (棚卸し)

`handleIgnoredErr` (`backend/internal/aws/errors.go:84-93`) の呼び出し箇所は次の 9 箇所で、4 リソース種別・6 フィールドに及ぶ。

| # | 箇所 | リソース種別 | 縮退するフィールド | JSON タグ |
| --- | --- | --- | --- | --- |
| 1 | `internal/aws/sqs.go:89` | SQS | `Tags` | `tags` |
| 2 | `internal/aws/waf.go:164` | WAF (REGIONAL) | `AssociatedCount` | `associated_count` |
| 3 | `internal/aws/waf.go:179` | WAF | `Tags` | `tags` |
| 4 | `internal/aws/waf.go:233` | WAF (CLOUDFRONT スコープ、クライアント生成失敗) | `AssociatedCount` | `associated_count` |
| 5 | `internal/aws/waf.go:240` | WAF (CLOUDFRONT スコープ、ListDistributions) | `AssociatedCount` | `associated_count` |
| 6 | `internal/aws/iam.go:131` | IAM (User) | `MFAEnabled` | `mfa_enabled` |
| 7 | `internal/aws/iam.go:142` | IAM (User) | `Groups` | `groups` |
| 8 | `internal/aws/iam.go:153` | IAM (User) | `Policies` | `policies` |
| 9 | `internal/aws/iam.go:195` | IAM (Role) | `Policies` | `policies` |
| 10 | `internal/aws/dynamo.go:99` | DynamoDB | `Tags` | `tags` |

WAF の `associated_count` (docs/issues/closed/0088 の起票元) は一覧の列として表示されるため症状が顕在化しやすいが、`Tags` (SQS/WAF/DynamoDB) と `Groups`/`Policies`/`MFAEnabled` (IAM) は現状 Drawer 側でのみ表示され、一覧列としては露出していない。

## 保留にした理由

本 issue は起票時点で以下が未確定であり、設計判断を要する。

- API レスポンス形状: フィールドごとの真偽フラグ (例 `associated_count_fetch_failed`) を各リソース型に個別追加する案と、汎用の `fetch_warnings: string[]` (縮退した JSON フィールド名の一覧) を追加する案のどちらを取るか。前者は型ごとに配線が増える一方、後者は `handleIgnoredErr` の呼び出し側で「元の err が非 nil で戻り値が nil (= 縮退が起きた)」ことを検知して収集する薄い仕組みで済み、将来フィールドが増えても API 形状を変えずに拡張できる。ただし後者は frontend 側で文字列キーと表示対象列を突き合わせる緩い対応になり、キー名の変更に追随できないリスクがある。
- frontend の表示方式: 値のグレーアウト、警告アイコン + tooltip、あるいは docs/issues/closed/0075 の DrawerError 方式を一覧行の集計フィールドに拡張するかの 3 案が未検討。加えて、対象フィールドの半数 (`Tags`/`Groups`/`Policies`/`MFAEnabled`) は現状一覧列に出ていないため、一覧行への表示が必要かどうか自体の判断も要る (Drawer 表示のみで足りる可能性がある)。
- 対象範囲: 9 箇所全てに同じ仕組みを適用するか、WAF の `associated_count` (症状が実際に報告された箇所) に限定し他は追って個別 issue にするかが未確定。

グローバル規約により、設計判断が必要で保留中の issue は issues/pending/ に置く。pending の issue は修正せずそのまま残す (close しない)。上記の棚卸しと保留理由の追記は修正に当たらない。

## 調査結果 (2026-08-05)

設計判断に必要な事実をコードから確認した。

まず箇所数について。
「調査結果 (棚卸し)」の表は 10 行あり、本文の「9 箇所」という記述は表の行数と食い違っている。
正しくは 10 箇所で、構造体とフィールドの組では 7 つ (`WAFResource` の `AssociatedCount` と `Tags`、`IAMResource` の `MFAEnabled` と `Groups` と `Policies`、`SQSResource` の `Tags`、`DynamoResource` の `Tags`) になる。

縮退時に各フィールドが取る値は次のとおり。

- SQS の `Tags`: 失敗時は空 map (`sqs.go:85` で初期化)。成功時は SDK の戻り値をそのまま代入する (`sqs.go:88`) ため、SDK が nil の map を返すと成功時だけ JSON が null になり、失敗時 (`{}`) と表現が逆転し得る。
- WAF の `AssociatedCount` (REGIONAL): リソース種別ごとの `ListResourcesForWebACL` (`waf.go:153-171`) の一部だけが失敗すると、合算 (`sumResourceARNs`、`waf.go:218-224`) は失敗分を 0 と数える。部分的に過小な値と完全な値が区別できない。
- WAF の `AssociatedCount` (CLOUDFRONT): クライアント生成または `ListDistributions` の失敗 (`waf.go:233`、`waf.go:240`) で、CLOUDFRONT スコープの全 ACL が 0 のままになる。
- IAM の `MFAEnabled`: 初期値 false のまま残る (`iam.go:128-131`)。JSON の値が「MFA 無効」と同一になり、レスポンス形状を変えない限り区別できない。
- IAM の `Groups` と `Policies`: 失敗時は nil スライスのままで JSON では null になる (`iam.go:137-153`、`iam.go:195`)。ただし成功して 0 件の場合も append が走らず nil のままなので、Go の値の時点で成功 0 件と失敗が同じ形になる。null と `[]` の使い分けでは区別できない。
- WAF と DynamoDB の `Tags`: 失敗時は非 nil の空 map (`waf.go:173-179`、`dynamo.go:92-99`)。成功時も map 変換関数を通るため非 nil で、JSON の表現は `{}` に揃っている。

frontend の受け皿は次のとおり。

- 一覧列は WAF の Associated (`columns.tsx:1358-1363`)、IAM の MFA (`columns.tsx:1417-1426`、有効は緑のチェック、無効は赤のバツ)、IAM の Policies と Groups の件数 (`columns.tsx:1434-1447`) が該当する。Tags の一覧列はどのサービスにも無い。
- Drawer では `DrawerTags.tsx` が「Tags (0)」と件数を表示し、`overviewRows.tsx` の wafOverviewRows (245-254 行) と iamOverviewRows (167-177 行) が該当行を持つ。
- 警告用のアイコン alertTriangle は `Icons.tsx:385` に定義済みで、現在は `ErrorBanner.tsx:17` だけが使う。title 属性によるツールチップは `DrawerECSTasks.tsx:120` に i18n 込みの先例がある。フィールド単位の警告表示プリミティブは無い。
- i18n の 14 ネームスペースに取得失敗を示す文言は無く、新規追加になる。

## 決定した設計

調査タスクの 3 点を次のとおり決めた。

**API レスポンス形状**: フィールドごとの真偽フラグを採用する。
対象の 4 構造体に `omitempty` 付きの bool フィールドを 7 つ追加する (`WAFResource` に `associated_count_fetch_failed` と `tags_fetch_failed`、`IAMResource` に `mfa_enabled_fetch_failed`、`groups_fetch_failed`、`policies_fetch_failed` の 3 つ、`SQSResource` と `DynamoResource` に `tags_fetch_failed`)。
フラグの意味は「対象フィールドの取得の一部または全部に失敗し、表示値が不完全または既定値である」とする。
WAF の REGIONAL スコープで一部のリソース種別だけ失敗した場合 (値はあるが過小) も true とする。
`omitempty` により失敗が無いレスポンスは現在と同一形状になり、後方互換を保つ。
縮退の検知は、`handleIgnoredErr` を包んで「縮退が起きたか」を bool で返すヘルパーで機械化し、キャンセル (context の打ち切り) は従来どおりエラーとして伝播してフラグの対象にしない。
併せて、`sqs.go:88` の成功パスを WAF や DynamoDB と同じ非 nil の map に正規化する (前述の null と `{}` の逆転の解消)。

採らなかった案は次の 2 つである。

- 汎用の `fetch_warnings: string[]`: frontend が文字列のフィールド名と表示列を突き合わせる緩い結合になり、フィールド名の変更に型検査で追随できない (「保留にした理由」に記載したリスク)。対象は現時点で 7 つであり、個別フラグの配線量は許容できる。
- null による表現 (ポインタ型や nil スライスで失敗を null にする): IAM の `Groups` と `Policies` は成功 0 件も nil になるため区別できず、WAF の REGIONAL の部分失敗 (値はあるが不完全) は null では表現できない。既存フィールドの型変更を伴い後方互換も失う。

**frontend の表示方式**: 警告アイコンと title ツールチップを採用する。

- 一覧列では、該当列 (WAF の Associated、IAM の MFA と Policies と Groups) の値の隣に alertTriangle を表示し、title 属性で取得失敗を説明する。IAM の MFA はフラグが立った行では赤のバツ (無効の断定) を表示せず、警告アイコンにする。
- Drawer では `overviewRows.tsx` の該当行に同じ警告を付す。`DrawerTags` はフラグが立った場合「Tags (0)」ではなく取得失敗の明示表示にする (docs/issues/closed/0075 の「エラーは空表示ではなく明示する」規則を Tags タブへ適用する)。
- `Tags`/`Groups`/`Policies`/`MFAEnabled` の一覧列は新設しない。現状の表示面 (一覧列にあるものは一覧列、Drawer だけのものは Drawer) に警告を重ねる。
- ツールチップの文言は ja の i18n リソースに新規追加する。AWS 由来メッセージの転記ではないため、docs/issues/closed/0066 の英語ハードコード方針の対象外とする。

採らなかった案は次の 2 つである。

- グレーアウトのみ: 灰色でも 0 は 0 と読めるため誤読が解消せず、IAM の MFA の赤バツによる「無効」の誤認も残る。
- DrawerError 方式の一覧拡張: DrawerError はサブタブ全体の失敗を面で表示する部品であり、フィールド単位の縮退とは粒度が合わない。

**対象範囲**: 10 箇所 (7 フラグ) 全てを 1 つの実装 issue で扱う。
フラグの配線、警告表示の部品、i18n の文言が共通の仕組みであり、`associated_count` だけ先行しても同じ部品を作ることになる。
「MFA 無効」の誤認という影響が最も大きい IAM を後回しにしない。
backend のフラグ追加と frontend の警告表示も分割しない。
フラグだけ先に close すると、レスポンスに現れた失敗を UI が無視する期間ができ、片方だけでは「ユーザーが判別できる」という本 issue の目的を満たさないためである。

決定した設計の実装は issue 0109 として起票した。

## 完了条件

- issue 0109 の完了後に確認する。
- docs/issues/0109 が close され、「決定した設計」に定めた 7 つのフラグと警告表示が 0109 の完了条件のとおり実装されている。
- 本 issue 自体はコードの変更を伴わない。

## 関連

- docs/issues/closed/0088: 本 issue の起票元。WAF の `associated_count` 修正時に、取得失敗時の縮退方針自体は変更しないことが確定している。
- docs/issues/0109: 決定した設計の実装 issue。

## 解決方法

本 issue はコードの変更を伴わない。要望 (取得失敗による 0 と真の 0 の判別) は docs/issues/closed/0109 の実装で満たされており、その観測結果を本 issue の完了条件の各行と突き合わせて確認した。

- 「issue 0109 の完了後に確認する」: docs/issues/0109 は docs/issues/closed/0109 として close 済みである (コミット 54b157c)。本確認はその後に行った。
- 「docs/issues/0109 が close され、『決定した設計』に定めた 7 つのフラグと警告表示が 0109 の完了条件のとおり実装されている」: 7 つのフラグは全て実装されている。`WAFResource` の `associated_count_fetch_failed` と `tags_fetch_failed` (`backend/internal/aws/waf.go`)、`IAMResource` の `mfa_enabled_fetch_failed` / `groups_fetch_failed` / `policies_fetch_failed` (`backend/internal/aws/iam.go`)、`SQSResource` の `tags_fetch_failed` (`backend/internal/aws/sqs.go`)、`DynamoResource` の `tags_fetch_failed` (`backend/internal/aws/dynamo.go`)。いずれも `omitempty` 付きで、失敗が無いレスポンスにフラグのキーは現れない。警告表示は一覧列 (`frontend/src/components/tables/columns.tsx` の WAF の Associated、IAM の MFA / Policies / Groups) と Drawer (`frontend/src/components/Drawer/overviewRows.tsx` の該当行、`frontend/src/components/Drawer/DrawerTags.tsx`) にあり、いずれも `frontend/src/components/primitives/FetchFailedWarning.tsx` を描画する。IAM の MFA はフラグが立った行で赤のバツを表示しない。0109 の完了条件の各行 (18 行) は、0109 の close 復旧時の swarm レビュー (観点 1) が実物 (テスト名、ファイルパスとシンボル名) で全行の充足を確認した。
- 「本 issue 自体はコードの変更を伴わない」: 本 issue の close にあたりコードは変更していない。0109 のコミット後の作業ツリーはクリーンである。

0109 のコミット時の pre-commit (`mise run fmt` / `mise run lint` / `mise run test`) が通過しており、実行冒頭のベースライン (失敗 0 件) からの新たな失敗は無い。
本 issue によるこのリポジトリへの変更は無いため、`CHANGES.md` へは追記しない。
