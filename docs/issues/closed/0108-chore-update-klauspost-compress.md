# github.com/klauspost/compress を v1.18.7 以上に更新する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「github.com/klauspost/compress を v1.18.7 以上に追随更新したい (issue 0095 のスコープ外として送り)」に対応する。

`backend/go.mod` (104 行) は `github.com/klauspost/compress v1.16.7` を間接依存として持つ。
`go mod graph` で確認すると、`cloud.google.com/go/bigquery v1.77.0` と `github.com/apache/arrow/go/v15 v15.0.2` の双方が直接要求している。
`mise run backend:lint` に含まれる govulncheck は GO-2026-5841 (`klauspost/compress/s2` の脆弱性) をモジュールレベルで報告する。
該当シンボルにはコードから到達しないため exit code には影響しないが、修正版は v1.18.7 で提供済みであり、更新すれば報告自体を消せる。
docs/issues/closed/0095 (grpc の脆弱性 GO-2026-6061 の解消) の実施時にこの報告が確認され、スコープ外として本 TODO に送られた。

同時に報告される GO-2026-5932 (`golang.org/x/crypto` の openpgp、GCP SDK 経由) は Fixed in N/A (修正版が存在しない) であり、バージョン更新では解消できないため本 issue のスコープ外とする。

## 対応方針

- docs/issues/closed/0095 と同じ手順で更新する。`go get github.com/klauspost/compress@latest` で引き上げ、`mise run backend:tidy` で整理する。
- 修正版の最低ラインである v1.18.7 に固定する案は採らない。最低ラインへの固定は次の脆弱性報告のたびに再固定の作業が要り、`@latest` で更新した docs/issues/closed/0095 の先例とも食い違う。`@latest` が解決したバージョンで問題が出ないことは、次項の個別検証で確認する。
- 間接依存のまま保つ。コードから直接 import しないため、`go.mod` の直接依存ブロックへ昇格させない (go mod tidy が `// indirect` を維持することで確認できる)。
- 検証は `mise run check` の一括実行ではなく、backend:build / backend:test / backend:lint の個別実行を先に行う。0095 で確認されたとおり、`mise run check` は先頭の fmt がツリーを書き換えるため、依存更新に起因する失敗と fmt に起因する差分が混ざると切り分けにくい。
- 実装時に `CHANGES.md` の `## develop` の `### misc` へ更新エントリを追記する (本 issue の起票時点では追記しない)。

## 完了条件

- `backend/go.mod` の `github.com/klauspost/compress` が v1.18.7 以上になっている。
- `mise run backend:lint` の govulncheck の報告に GO-2026-5841 が含まれない。
- `github.com/klauspost/compress` が間接依存 (`// indirect`) のままである。
- `golang.org/x/crypto` のバージョンには変更を加えない (GO-2026-5932 は Fixed in N/A のためスコープ外)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0095: 本 issue の起票元。更新手順 (go get → tidy → 個別検証) のテンプレート。

## 解決方法

- `go get github.com/klauspost/compress@latest` で `github.com/klauspost/compress` を v1.16.7 から v1.19.2 へ更新し、`mise run backend:tidy` で整理した。`// indirect` は維持されている。
- 更新後に `govulncheck -show verbose ./...` を実行し、報告から GO-2026-5841 が消えたことを確認した。残る報告はスコープ外の GO-2026-5932 (golang.org/x/crypto の openpgp、Fixed in N/A) の 1 件のみで、`golang.org/x/crypto` のバージョン (v0.53.0) には変更を加えていない。
- 方針どおり `mise run backend:build` / `mise run backend:test` / `mise run backend:lint` を個別に実行し、いずれも通ることを確認した。
- `mise run check` が通ることを確認した。
