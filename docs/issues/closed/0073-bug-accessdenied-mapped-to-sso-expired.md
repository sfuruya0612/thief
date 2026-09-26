# AWS の AccessDenied が SSO 期限切れ (401) に誤マップされるのを修正する

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 症状

権限不足 (AccessDenied) の AWS API エラーが HTTP 401 のコード `SSO_TOKEN_EXPIRED` として返り、UI が「SSO の再ログイン」を促すバナーを表示する。
実際には再ログインしても解決せず、利用者は権限不足に気付けない。
docs/issues/0075 (WAF ルール表示) のレビューで見つかった、コード上の帰結として特定した問題であり、実環境での観測はまだ無い (TODO.md 由来ではない)。

## 再現手順

1. 検証用プロファイルの IAM エンティティに、いずれかの AWS API を拒否するポリシーを付与する (例: `wafv2:ListWebACLs` の Deny)。
2. thief でそのプロファイルの該当サービス (例: WAF) の一覧を開く。
3. ブラウザ開発者ツールの Network タブで、該当リクエストのレスポンスの status と `code` を確認する。
4. 期待: 403 とコード `ACCESS_DENIED` が返り、権限不足であることが分かるエラー表示 (`ErrorBanner`) が出る。実際: 401 とコード `SSO_TOKEN_EXPIRED` が返り、`SSOExpiredBanner` が表示され、SSO 再ログインを促される。

## 原因

`backend/internal/api/errors.go` の `writeAWSError` は `IsSSOTokenExpired` だけを見てから 500 に落とす。

```go
func writeAWSError(w http.ResponseWriter, err error) {
	if awsinternal.IsSSOTokenExpired(err) {
		// ... 401 SSO_TOKEN_EXPIRED
	}
	// ... 500
}
```

`IsSSOTokenExpired` (`backend/internal/aws/errors.go`) の部分一致リストに `not authorized` が含まれるため、AccessDenied 系のエラーメッセージが SSO 期限切れとして判定される。
同じファイルの `writePricingError` は `IsAccessDenied` を `IsSSOTokenExpired` より先に判定する順序になっており、「Order matters」の趣旨のコメントも付いている。
`writeAWSError` にはこの順序がない。

## 修正方針

- `writeAWSError` に、`IsSSOTokenExpired` より先に `IsAccessDenied` (`backend/internal/aws/errors.go`、smithy コード `AccessDeniedException` / `AccessDenied` / `UnauthorizedOperation` の 3 つを判定) の分岐を追加する。
- AccessDenied は HTTP 403、エラーコード `ACCESS_DENIED`、メッセージは `err.Error()` で返す。`writePricingError` の `PRICING_ACCESS_DENIED` はコードも文言も pricing 専用のため流用しない (揃えるのは判定順とステータスだけ)。
- frontend は変更しない。各ビューは `SSO_TOKEN_EXPIRED` のコード一致でのみ `SSOExpiredBanner` を出し、それ以外の `ApiError` は `ErrorBanner` (ステータス + コード + メッセージを表示) に落ちる既存経路がそのまま使える。
- 次は本 issue では扱わない。
  - `IsSSOTokenExpired` の部分一致リスト (`not authorized` / `ForbiddenException` / `UnauthorizedException`) 自体の見直し。修正後も残る影響は「未確定論点」に書く。
  - `writeInternalFromError` を使う経路 (cost / bigquery / datadog / tidb) での権限エラーの 403 化。これらは 401 誤マップは起こさず (500 になる)、症状が異なる。
  - `IsThrottled` (429) 分岐の `writeAWSError` への追加。誤マップの修正ではなく挙動の拡張になる。
  - 更新系 mutation の失敗表示 (`DrawerValueEditor` の `saveError` 経由) の改善。本 issue は backend のマッピング修正のみで、frontend の表示側は変更しない (表示側の変更は docs/issues/0075 が `DrawerError` への置き換えで行う)。

## 完了条件

- 既存の `backend/internal/api/errors_test.go` (現在のテストは `TestWriteGCPError` のみで `writeAWSError` のテストは無い) に `TestWriteAWSError` を追加し、`AccessDeniedException` / `AccessDenied` / `UnauthorizedOperation` のエラーが 403 とコード `ACCESS_DENIED` で返ること、SSO 期限切れのエラーが引き続き 401 と `SSO_TOKEN_EXPIRED` で返ることを status と code の双方で検証している。
- `views/AccountView.test.tsx` を新設し (両バナーの出し分けは `frontend/src/views/AccountView.tsx` の `ServicePanel` が行い、既存テストは無い。新設には `useResources` / `useCost` のモックが要る)、403 `ACCESS_DENIED` の `ApiError` では `SSOExpiredBanner` が表示されず、`ErrorBanner` にステータスとコードとメッセージが表示されることを検証している。
- `IsSSOTokenExpired` の部分一致リストの見直し、`writeInternalFromError` 経路、429 分岐の追加は本 issue では扱わない。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 未確定論点

- `IsSSOTokenExpired` の部分一致リストは、SSO 期限切れと権限不足の両方で現れうる文字列 (`not authorized` / `ForbiddenException` / `UnauthorizedException`) を含む。smithy コードが `AccessDeniedException` / `AccessDenied` / `UnauthorizedOperation` 以外の権限エラー (例: `ForbiddenException` を返すサービス) は、本修正後も 401 に写る可能性が残る。切り分けには実環境での観測が要るため、観測されたら別途起票する。
- `writeInternalFromError` 経路の権限エラーは 500 になるが、実環境で問題として観測された事例が無いため、本バッチでは起票しない。

## 関連

- docs/issues/0075 (WAF ルール表示): 発見元。`wafv2:GetWebACL` の新設経路で顕在化しやすいため、本 issue を先に (番号順で) 修正する。
- docs/issues/0078 (重複の共通化): frontend の SSO 期限切れ判定の共通化。本 issue は backend のエラーマッピングを扱い、対象が異なる。

## 解決方法

- `backend/internal/api/errors.go` の `writeAWSError` に、`IsSSOTokenExpired` より先に `IsAccessDenied` の分岐を追加した。AccessDenied 系 (smithy コード `AccessDeniedException` / `AccessDenied` / `UnauthorizedOperation`) は HTTP 403、エラーコード `ACCESS_DENIED`、メッセージ `err.Error()` で返す。判定順の根拠 (緩い部分一致より先に厳密な smithy コード判定を走らせる) を godoc コメントに記載した。
- `backend/internal/api/errors_test.go` に `TestWriteAWSError` を追加した。AccessDenied 3 コード + `%w` ラップの 4 ケースが 403 `ACCESS_DENIED`、SSO 期限切れ 2 ケースが 401 `SSO_TOKEN_EXPIRED`、その他が 500 `INTERNAL_ERROR` になることを status と code の双方で検証している。
- `frontend/src/views/AccountView.test.tsx` を新設した。`useResources` / `useCost` を部分モックし、403 `ACCESS_DENIED` の `ApiError` では `SSOExpiredBanner` が表示されず `ErrorBanner` にステータス・コード・メッセージが表示されること、401 `SSO_TOKEN_EXPIRED` では `SSOExpiredBanner` が表示されること、エラー無しではどちらも出ないことを検証している。frontend の実装コードは変更していない。
- `CHANGES.md` の `## develop` に `[FIX]` エントリを記載した。
- `mise run check` 通過を確認した。
