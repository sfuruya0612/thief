# CloudFront の一覧を 5 列にして Alternate domains に代替ドメイン名を表示する

Created: 2026-07-27
Completed: 2026-07-27
Model: Claude Fable 5 / Claude Sonnet 5

## 背景

TODO.md の次の項目に対応する (原文の Destibution, Destribution は Distribution の誤記と解釈する)。

> CloudFront の一覧のカラムは Destibution, State, Domain, Alternate domains, Origins にして欲しい
> - Alternate domains に Destribution の値が入ってしまっていそう

現状の `cloudfrontColumns` (`frontend/src/components/tables/columns.tsx:968-1021`) は 7 列で、`id` (ヘッダ `Distribution`、14%)、`state` (11%)、`domainName` (ヘッダ `Domain`、22%)、`name` (ヘッダ `Alternate domains`、20%)、`origins` (19%)、`enabled` (7%)、`priceClass` (7%) の順に並ぶ。

TODO の引用の 2 行目 (子行の観測) は正しい。
`Alternate domains` 列は `key: 'name'` (`columns.tsx:993-998`) で `CloudFrontRow.name` を表示しているが、backend の `cloudfrontFromSummary` (`backend/internal/aws/cloudfront.go:77-97`) は `name` に Comment を入れ、Comment が空なら Distribution ID で埋める。

```go
name := ptrStr(d.Comment)
if name == "" {
    name = ptrStr(d.Id)
}
```

つまり `Alternate domains` の見出しの下に Comment または Distribution ID が表示されており、代替ドメイン名 (CNAME) はどこにも表示されていない。
そもそも backend の `CloudFrontResource` (`backend/internal/aws/cloudfront.go:13-22`) に aliases に相当するフィールドが無い。
SDK の `DistributionSummary` (aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:2552`) は `Aliases` を含んでおり、`ListDistributions` のレスポンスから追加の API 呼び出しなしで取得できる。
Drawer の Overview (`frontend/src/components/Drawer/overviewRows.tsx:195`) も同じ `r.name` を `Alternate domains` として表示しており、同じ誤りがある。

列構成の変更と Alternate domains の誤表示の修正は、TODO で 1 項目とその子行 (観測) として書かれた同一の要望であり、5 列への再構成が `aliases` フィールドの追加を前提とする一方向の依存もあるため 1 issue で扱う。
カテゴリは主目的の列構成変更に合わせて feat とする。
TODO の列挙を列の全集合 (残り 2 列を外す) と読む解釈は、2026-07-27 にユーザーへ確認し、2 列を外す意図で確定した。
feat の issue が `[FIX]` エントリを併記する前例は issues/closed に無いが、誤表示の修正がリリースノートから読み取れることを優先し、CHANGES.md には `[UPDATE]` と `[FIX]` の 2 エントリを書く (完了条件に記載)。
`aliases` フィールドの追加は誤表示修正の手段のため、単独の `[ADD]` エントリは書かない。

## 目的

CloudFront の一覧が Distribution, State, Domain, Alternate domains, Origins の 5 列になり、Alternate domains 列に代替ドメイン名 (CNAME) が表示される。

## 設計判断

- backend は `CloudFrontResource` に `aliases []string` (JSON タグ `aliases`) を追加し、`cloudfrontFromSummary` で `d.Aliases.Items` から写す。`DistributionSummary.Aliases` は SDK 上 required member のため実レスポンスでは非 nil で、代替ドメイン名が無い場合の実際の形は `Items` が空の非 nil 構造体になる。`Aliases` 自体が nil のケースのガードは防御として入れる。
  - nil の扱いは `origins` と同じにする: 収集は `append` のみで行い、0 件なら nil のまま JSON では null になる。frontend の Raw 型を `aliases: string[] | null` とし、`cloudfrontFromRaw` で `raw.aliases ?? []` と写す。
    - 却下案: backend で空スライスに正規化して常に `[]` を返す。既存の `origins` (nil のまま返して frontend で吸収する) と非対称になるため却下。
  - 追加の AWS API 呼び出しと権限は不要 (`ListDistributions` のレスポンスに含まれる)。
  - backend のリソースキャッシュはプロセス内メモリ (`backend/internal/cache/cache.go`) で永続化されないため、フィールド追加にデータ移行の考慮は不要。
- `name` フィールド (Comment 由来) は変更しない。CLI の表示 (`backend/internal/aws/torow.go:66-68`) と Drawer のヘッダタイトル (`frontend/src/components/Drawer/Drawer.tsx:235-236` が `resource.name` を表示) が使っており、一覧から `name` 列を外しても Comment の確認場所は Drawer に残る。
  - 却下案: `name` の Distribution ID フォールバックをやめる。一覧の行タイトルとしての用途が CLI に残っており、本 issue のスコープ (列構成) を超えるため却下。
- frontend は `CloudFrontRaw` に `aliases: string[] | null`、`CloudFrontRow` に `aliases: string[]` を追加し、`cloudfrontFromRaw` (`frontend/src/lib/normalize.ts:562-573`) で写す。
- `cloudfrontColumns` を次の 5 列にする: `id` (ヘッダ `Distribution`)、`state` (ヘッダ `State`)、`domainName` (ヘッダ `Domain`)、`aliases` (ヘッダ `Alternate domains`)、`origins` (ヘッダ `Origins`)。幅は 16, 10, 25, 25, 24 (合計 100%) を初期値の案とする。
  - TODO の文言を列の全集合の指定と解釈し、`enabled` と `priceClass` は一覧から外す (背景に記載のとおり 2026-07-27 にユーザーへ確認済み)。TODO の書き手は WAF の項目 (docs/issues/0087) で既存 7 列の全てを列挙して順序を指定しており、本項目で 5 列しか挙げないのは残り 2 列を外す意図と解釈する。現行 7 列の先頭 5 列 (`id`, `state`, `domainName`, `name`, `origins`) は TODO の列挙と順序まで一致しており、要望が並べ替えではなく末尾 2 列の削除と Alternate domains の値の是正であることを裏付ける。外す 2 列はどちらも Drawer の Overview (`overviewRows.tsx:191-200`) に残っており、確認手段は失われない。
  - 一覧から外した列は列フィルタとソートでも使えなくなる。TODO の列挙を全集合の指定として尊重し、次の損失を受容する。
    - `enabled` は `state` と別軸の絞り込みのため、この軸での絞り込みができなくなる。
    - `name` 列を外すと Comment での絞り込みと並べ替えができなくなる (FacetBar は Env, state, region, Team の 4 軸のみで、フリーテキストの代替は無い)。CNAME 未設定のディストリビューションは一覧上 Distribution ID と CloudFront ドメインだけで識別することになり、Comment の表示面は Drawer ヘッダのみになる。
    - `Alternate domains` 列は `name` (string) から `aliases` (配列) に変わるためソートが効かなくなる (`DataTable` の `sortValue` は string / number / boolean 以外を undefined にするため。既存の `origins` 列と同じ扱い)。
  - 却下案: `enabled` と `priceClass` を残して 7 列にする。上記の全集合解釈と矛盾するため却下。
  - 幅の配分は、現行で最大幅の `domainName` (22%) と同水準をドメイン系 2 列に与え、`origins` に現行値 (19%) より広い幅を与える案である。個々の幅は実装時に確定してよく、完了条件は列の集合と順序、合計 100% で判定する (実装の値を写すだけの検証を完了条件にしない)。
- `aliases` セルは `origins` セルと同じ表示方式 (カンマ結合、空ならダッシュ) にする。`filterValue` は与えず、列フィルタは既定動作 (`String(row[key])` の文字列化。配列はスペース無しのカンマ結合になる) に委ねる。表示の `', '` 結合と絞り込みの一致文字列が異なる点は既存の `origins` 列と共通の既知差で、両列まとめての是正は本 issue では扱わない。
- 既知の限界: マルチテナントディストリビューション (`ConnectionMode` が `tenant-only` の本体) では Alternate domains 列がダッシュになり得る。`DistributionConfig.Aliases` のドキュメント (aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:2152-2160`) が standard distribution 専用のフィールドであることを明記しており、マルチテナント構成のドメインはディストリビューションテナント側に設定されるため、本体の `Aliases` からは得られない。テナントのドメインまで表示するには `ListDistributionTenants` 系の追加呼び出しと新権限が要るため扱わない (docs/issues/0088 がテナントの関連付けを数えないのと同じ判断)。
- Drawer の Overview の `Alternate domains` 行 (`overviewRows.tsx:195`) を `r.name` から `r.aliases` のカンマ結合に直す。あわせて Overview に Comment の行は追加しない (Drawer ヘッダに表示済みのため)。
- CLI の列構成は変更しない (TODO は Web の一覧の要望であり、CLI の表は別系統のため)。

## 完了条件

- `cloudfrontFromSummary` の `aliases` の検証が `backend/internal/aws/cloudfront_test.go` にあり、`Aliases` あり (複数件) で `aliases` に写るケースと、`Aliases` が非 nil で `Items` が空 (required member を持つ実レスポンスの 0 件の形) のとき `aliases` が nil になるケースと、`Aliases` が nil で `aliases` が nil になるケースを含む (長さ 0 の検証では nil と空スライスを区別できず、nil のまま返して JSON で null にするという設計判断を検証したことにならない)。既存の `TestCloudfrontFromSummary` (`backend/internal/aws/cloudfront_test.go:10`) のテーブルは state 正規化専用の構造 (`{name, status, want string}`) のため変更せず、テスト関数を新設してフィールド単位のアサーションで検証する (docs/issues/0090 の `behaviors` の検証も同じ方式で別関数を新設し、互いのフィールド追加でテストが壊れないようにする)。
- `CloudFrontResource` の `json.Marshal` 出力に `aliases` キーが現れ、0 件のとき値が `null` になることを検証するテストがある (`TestWAFResourceJSONHasDescription` (`backend/internal/aws/waf_test.go:346`) と同じ方式)。
- `frontend/src/lib/normalize.test.ts` に `cloudfrontFromRaw` の describe を新設し、`aliases` が null のとき空配列になるケースと、値があるとき写るケースがある。検証はフィールド単位のアサーションで行う (docs/issues/0090 のフィールド追加でテストが壊れないように)。
- `cloudfrontColumns` の `key` の並びが `id`, `state`, `domainName`, `aliases`, `origins` の 5 列でこの順に完全一致することと、対応する `header` が `Distribution`, `State`, `Domain`, `Alternate domains`, `Origins` であることと、`width` の合計が 100% であることを `frontend/src/components/tables/columns.test.tsx` で検証している (個々の幅の値は検証しない)。
- `cloudfrontColumns` の `aliases` セルが複数値を `', '` (カンマ + 半角スペース) の結合で表示し、空配列でダッシュになることを `columns.test.tsx` で検証している (`columns.test.tsx:30-44` の既存方式に合わせる)。
- `frontend/src/components/Drawer/overviewRows.test.tsx` に `cloudfrontOverviewRows` の describe を追加し、`Alternate domains` 行が `aliases` のカンマ結合を表示するケースと、空配列でダッシュになるケースがある。あわせて `Enabled` と `Price class` の行が Overview に残っていることを検証する。
- CLI の列構成の変更と、`name` フィールド (Comment 由来、Distribution ID フォールバックを含む) の変更は本 issue では扱わない。`aliases` 列の列フィルタは既定動作に委ねるため (設計判断に記載)、表示のカンマ結合と絞り込みの文字列化の差を検証するテストも書かない。
- `CHANGES.md` の `## develop` に `[UPDATE]` (列構成の変更) と `[FIX]` (Alternate domains の誤表示修正) の 2 エントリを記載し、それぞれに担当者行を付ける。2 エントリは隣接させず、それぞれ既存の `[UPDATE]` 群と `[FIX]` 群へ、種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置に挿入する。
- `mise run check` が通過する。

## 関連

- docs/issues/0087 (WAF の列順変更): 同じ `frontend/src/components/tables/columns.tsx` と `columns.test.tsx` を触る。対象の配列が異なるため、番号順に実装すれば衝突しない。
- docs/issues/0090 (CloudFront の Behaviors タブ): 同じ `backend/internal/aws/cloudfront.go`、`frontend/src/types/aws.ts` の CloudFront の型定義、`frontend/src/lib/normalize.ts`、`frontend/src/components/tables/columns.tsx` を触るため同時進行はできない。本 issue の完了後に docs/issues/0090 に着手する。
- docs/issues/0086 (WAF ルール詳細): 同じ `frontend/src/types/aws.ts`、`frontend/src/lib/normalize.ts`、`normalize.test.ts` を触る。編集箇所は型と関数の単位で独立しており、番号順に実装すれば衝突しない。
- docs/issues/0088 (WAF の Associated 修正): `backend/internal/aws/cloudfront.go` を変更せず既存の `newCloudFrontClient` と `ListDistributions` を呼ぶだけのため、本 issue と衝突しない。
- docs/issues/0086, docs/issues/0087, docs/issues/0088, docs/issues/0090: `CHANGES.md` の `## develop` は 5 issue 全てが変更する。番号順に直列で実装し、各 issue のエントリを種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入すれば衝突しない (本 issue のみ `[UPDATE]` と `[FIX]` の 2 エントリを書く)。

## 解決方法

- `backend/internal/aws/cloudfront.go`
  - `CloudFrontResource` に `Aliases []string \`json:"aliases"\`` を追加 (`DomainName` の直後、`Origins` の直前)。
  - `cloudfrontFromSummary` で `d.Aliases` が非 nil のときのみ `Items` を `append` し、`nil` または `Items` が空なら `aliases` を `nil` のまま返す (`Origins` と同じ nil 温存方式)。
- `backend/internal/aws/cloudfront_test.go`
  - `TestCloudfrontFromSummaryAliases` を新設 (複数件が写るケース、`Items` が空の非 nil `Aliases` で nil になるケース、`Aliases` が nil で nil になるケースの 3 パターン、nilness と要素を個別比較)。既存の `TestCloudfrontFromSummary` は変更していない。
  - `TestCloudFrontResourceJSONHasNullAliasesWhenEmpty` を新設し、`aliases` フィールド未設定時の JSON が `"aliases":null` を含むことを検証。
- `frontend/src/types/aws.ts`: `CloudFrontRaw` に `aliases: string[] | null`、`CloudFrontRow` に `aliases: string[]` を追加。
- `frontend/src/lib/normalize.ts`: `cloudfrontFromRaw` に `aliases: raw.aliases ?? []` を追加 (`origins` と同じ変換)。
- `frontend/src/components/tables/columns.tsx`: `cloudfrontColumns` を `id`(16%) / `state`(10%) / `domainName`(25%) / `aliases`(25%、新設、カンマ結合 + `Dash` フォールバック) / `origins`(24%) の 5 列に絞り込み、`name` / `enabled` / `priceClass` の列を削除。
- `frontend/src/components/Drawer/overviewRows.tsx`: `cloudfrontOverviewRows` の `Alternate domains` 行を `r.name` から `r.aliases.join(', ') || dash` に変更。`Enabled` と `Price class` の行は変更せず維持。
- テスト追加: `normalize.test.ts` (`cloudfrontFromRaw` の null→[] / 値あり)、`columns.test.tsx` (`cloudfrontColumns` の列順・ヘッダ・幅合計 100%・`aliases` セルのカンマ結合/ダッシュ表示)、`overviewRows.test.tsx` (`cloudfrontOverviewRows` の `Alternate domains` 行のカンマ結合/ダッシュ表示、`Enabled`/`Price class` 行の残存確認)。
- `mise run check` は fmt / lint (frontend 0 エラー・既存警告のみ、backend govulncheck 0 vulnerability) / test (frontend 69 ファイル 604 件全通過、backend 全パッケージ ok) が全て通過した。
- `CHANGES.md` の `## develop` に `[UPDATE]` (5 列への絞り込み) と `[FIX]` (Alternate domains の誤表示修正) の 2 エントリを、非隣接かつ種別順を保つ位置に追加した。
- CLI の列構成、`name` フィールドの意味、`aliases` 列フィルタの既定挙動はいずれも変更していない (完了条件どおり範囲外)。
