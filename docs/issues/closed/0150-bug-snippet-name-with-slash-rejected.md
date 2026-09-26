# スラッシュを含む名前のスニペットの保存が 400 で拒否される問題を解消する

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-25

## 症状

docs/issues/TODO.md の次の項目に由来する。

> BigQuery の Snippets 保存で `virtual_money_issue_refund / virtual_money_use_refund（取消・返金）` を保存しようとするとサーバーが 400 エラーになっている
>     確認はしていないがおそらく Athena でも同様の現象になると思われる

BigQuery のクエリエディタで、スニペット名に `/` を含む文字列 (上記の例では ` / ` の 3 文字) を指定して保存すると、backend が HTTP 400 を返して保存に失敗する。応答ボディはコードから次のとおり特定できる (実行による確認は未実施): `{"error": "invalid snippet name: must not contain path separators", "code": "BAD_REQUEST"}`。

TODO の子行は「おそらく Athena でも同様」と推測しているが、これは事実である。名前の検証 (`validateName`) は athena / bigquery 共通の `internal/snippet` パッケージにあり (backend/internal/snippet/snippet.go:60-71)、サービスによる分岐は無い。

frontend では、保存失敗のエラーは `useServerSnippets` (frontend/src/components/query/useServerSnippets.ts:41) が集約し、Snippets パネル内の `ErrorBanner` (frontend/src/views/nonaws/BigQueryView.tsx:353、frontend/src/views/AthenaView.tsx:462) に英語メッセージのまま表示される。

## 再現手順

1. `mise run backend:run` で API サーバを起動する。
2. 次のリクエストを送る。

   ```
   curl -sS -X POST http://127.0.0.1:8089/api/snippets/bigquery \
     -H 'Content-Type: application/json' \
     -d '{"name":"virtual_money_issue_refund / virtual_money_use_refund（取消・返金）","sql":"SELECT 1"}'
   ```

   観測される結果: HTTP 400 と `{"error":"invalid snippet name: must not contain path separators","code":"BAD_REQUEST"}` が返る。
3. UI 経由の再現 (手順 2 とは独立の経路): BigQuery ビュー (または Athena ビュー) のツールバーの Snippets ドロップダウンから「現在のクエリを保存」を選び、`window.prompt` (frontend/src/views/nonaws/BigQueryView.tsx:156、frontend/src/views/AthenaView.tsx:190) に上記の名前を入力する。Snippets パネルの `ErrorBanner` に同じメッセージが表示され、スニペットは一覧に追加されない。

## 原因

スニペットは名前をそのままファイル名に使って `<baseDir>/<service>/<name>.sql` へ保存される (`Store.path`、backend/internal/snippet/snippet.go:77-79)。このため `validateName` (snippet.go:60-71) が、パス区切り文字 (`/`、`\`)、NUL、先頭ドット、空文字、128 バイト超の名前を `ErrInvalidName` で拒否する。該当の名前は ` / ` を含むため拒否され、`writeSnippetError` (backend/internal/api/handlers_snippets.go:52-61) が `ErrInvalidName` を 400 BAD_REQUEST にマップする。

検証自体はパストラバーサル防止として正当だが、名前とファイル名を同一視しているため、ファイル名に使えない文字を含む正当なスニペット名が保存できない。

## 修正方針

名前とファイル名の対応にエンコードを導入し、ファイル名に使えない文字を含む名前でも保存できるようにする。

- 保存時: ファイル名として安全でない文字 (`/`、`\`、NUL)、先頭のドット、およびエンコードに用いる `%` 自体をパーセントエンコードしてファイル名を生成する。16 進表記は大文字 (`%2F` 形式) に統一する。それ以外の文字 (全角文字を含む) はエンコードせず、ファイル名の可読性を保つ。
- 一覧時: ファイル名 (拡張子 `.sql` を除いた部分) をデコードして名前に戻す。ただしデコードは、デコード結果を再エンコードすると元のファイル名に一致する場合 (正規形の場合) に限って適用し、一致しないファイル名 (デコードできない `%` 並び、16 進が小文字の `%2f` 形式、エンコードなしで `%` を含む手動配置のファイル名) はエラーにせず、ファイル名をそのまま名前として扱う。この規則により、どのファイルでも「一覧に出た名前で削除できる」ラウンドトリップが成り立つ。
- 一覧時の正規形判定の帰結として、手動配置のファイル名がたまたま正規形のエンコード列を含む場合 (例: `100%2Foff.sql`) は、一覧の名前がデコード結果 (`100/off`) になる。この名前での削除と上書きは成立するため、表示名がファイル名の字面と異なることは許容する。
- 削除時: 名前をエンコードしたファイル名をまず探し、無ければ名前をそのままファイル名とみなす後方互換のパスも探す (手動配置された非正規形のファイル名に対応する。この場合も名前にパス区切りを含まないことは検証する)。
- 名前の検証は「空でないこと」「バイト長の上限」に縮小し、文字種の制限を撤廃する。長さの上限はエンコード後のファイル名に対して適用する (ファイルシステムのファイル名長制限が対象とするのはエンコード後の長さであるため)。エンコード後の長さで拒否する場合、エラーメッセージにはエンコード後のファイル名長の制限であることを明示する。
- 手動で配置した .sql ファイル (パッケージコメント snippet.go:1-3 が明記する利用形態) は、`%` を含まなければ従来どおりファイル名がそのまま名前になる。現行の `validateName` は `%` を禁止していないため `%` を含む名前の既存スニペットは存在しうるが、非正規形ならそのままの名前として扱われ、正規形なら上記のとおりデコード結果の名前になる。いずれの場合も一覧、削除、上書きは成立するため、移行処理は追加しない。

検討済みの境界条件。

- 名前はバイト列のまま扱い、Unicode 正規化 (NFC / NFD) は行わない。macOS の APFS はファイル名の正規化を保存するため、保存した名前は同じバイト列で一覧に戻る。ファイル名を NFD へ変換するファイルシステム上での表示名の揺れは扱わない (完了条件の「扱わない範囲」に明記)。
- 名前が `.sql` で終わる場合、ファイル名は `<name>.sql.sql` になり、一覧時の拡張子除去で元の名前に戻る。現行の `List` (snippet.go:96) と同じ扱いで問題ない。
- 名前の前後の空白が `handleSnippetSave` (backend/internal/api/handlers_snippets.go:29) でトリムされる既存挙動は変更しない。
- ファイルシステムが大文字小文字を同一視する環境 (macOS の APFS 既定) での名前の衝突 (`Foo` と `foo` が同一ファイルになる) は現行仕様と同じであり、本 issue では扱わない。エンコードの 16 進を大文字に統一するのは、この環境で `%2f` と `%2F` が同一ファイルに解決されても正規形判定が揺れないようにするためでもある。
- 一時ファイル (`os.CreateTemp(dir, ".tmp-*")`、snippet.go:136) は拡張子が `.sql` でないため一覧から除外され、エンコード導入後も名前と衝突しない。
- (実装時の追記) 手動配置により正規形ファイル (例: `50%25off.sql`、表示名 `50%off`) と非正規形ファイル (例: `50%off.sql`、表示名 `50%off`) が同居すると、一覧の表示名が衝突する。修正方針の境界条件は `/` を介した衝突をファイルシステム上作成できないため問題にならないとしたが、`%` を介した衝突は作成できる。この場合も一覧は両ファイルを返し、その名前の削除は正規形 → 非正規形の順に 1 回 1 ファイルずつ成立するため (行き止まりにならない)、実装は変えず挙動をテストで固定する。名前からファイルを一意に特定できないのは、衝突するファイル名を手動で配置した場合に限られる。
- (実装時の追記) 手動配置のファイル名が `\` を字面に含む場合 (POSIX では合法なファイル名)、一覧にはファイル名のまま載るが、API からの削除は成立しない。削除時の後方互換パスは修正方針のとおり名前にパス区切りを含まないことを検証し、`\` は Windows でのパストラバーサル (`a\..\..\evil` 形式) を防ぐため防御的にパス区切りとして扱うためである。正規の Save 経路では `\` はエンコードされるため、この制限は手動配置ファイルだけに当たる。「どのファイルでも一覧に出た名前で削除できる」というラウンドトリップの記述は、この 1 点についてパス区切りの検証が優先される。
- (実装時の追記) エンコード後のバイト長上限の検証は Save だけに適用し、Delete では空でないことだけを検証する。一覧はファイル名の長さで除外しないため、手動配置された上限超のファイル名も一覧に載る。Delete が長さで拒否すると、その一覧に出た名前で削除できずラウンドトリップが壊れるため。List も同じ理由で長さの検証を行わず、隠しファイル (先頭ドット。Save の一時ファイル `.tmp-*` を含む) の除外だけを行う。Delete の長さ検証の撤廃により、ファイルシステムのファイル名長制限を超える名前 (一覧を経由しない直接のリクエストだけが取りうる) では os.Remove が ENAMETOOLONG を返すが、そのような名前のファイルは存在しえないため ErrNotFound (HTTP 404) に写像する。旧実装の 400 からは応答が変わるが、サーバエラー (500) にはしない。

採らなかった案とその却下理由。

- frontend で `/` を含む名前を事前に拒否し、明確なメッセージを表示する案。TODO の要望は当該の名前で保存することであり、名前の文字種を制限したままでは解決にならない。
- 名前とファイル名の対応表 (インデックスファイル) を別に持つ案。「手動で配置した .sql ファイルもそのまま一覧に載る」という `internal/snippet` パッケージの設計 (snippet.go:1-3) が壊れ、インデックスと実ファイルの不整合という新しい失敗モードを持ち込む。
- `/` を全角スラッシュ等の類似文字へ置換して保存する案。保存した名前と一覧に表示される名前が変わりラウンドトリップせず、置換後に同名になる別名同士の衝突も起こる。
- 名前全体を Base64 等で不可逆に見える形へエンコードする案。ラウンドトリップの実装は単純になるが、ファイル名から中身を推測できる可読性が失われ、手動でファイルを配置・編集する利用形態 (snippet.go:1-3) と両立しない。
- 非正規形のファイル名の受け入れをやめ、正規形のエンコード済みファイル名だけを有効とする案。正規形判定が不要になり実装は単純になるが、エンコード規則を知らずに手動で配置されたファイル (`%` を含むファイル名) が一覧から消える。手動配置ファイルの共存が要件である以上、正規形判定の複雑さは要件由来のものとして受け入れる。

DELETE 経路への影響: 削除は `DELETE /api/snippets/{service}/{name}` (backend/internal/api/routes.go:122) で名前をパスセグメントとして受け取り、frontend は `encodeURIComponent(name)` 済みである (frontend/src/api/endpoints.ts:873)。`/` を含む名前は `%2F` としてパスに乗る。`http.ServeMux` が `%2F` を含むセグメントを 1 セグメントとして扱うことは Go 1.26.6 のソースで確認済みである: ルーティングは `r.URL.EscapedPath()` を対象にし (net/http/server.go:2660)、`%2f` はセパレータ扱いされずに保持され (server.go:2549-2550 の ServeMux ドキュメント)、ワイルドカードのマッチ値はセグメント単位でデコードされる (net/http/routing_tree.go:214 の `pathUnescape`)。したがって `r.PathValue("name")` は `/` を含む元の名前を返し、DELETE の受け渡し方式の変更は不要である。

追加の API 呼び出しや権限は不要である。変更は backend の `internal/snippet` パッケージとそのハンドラに閉じ、frontend の変更は不要である (エンコードは backend 内部の関心事で、API 上の名前は生の文字列のまま。DELETE のパスセグメント渡しも上記のとおりそのまま使える)。

## 完了条件

- 再現手順 2 の curl が 200 を返し、レスポンスの `name` が入力の名前と一致する。保存の成功は単体テストでも検証するが、この curl は実施して結果を issue に記録する。
- 保存後の `GET /api/snippets/bigquery` の一覧に、エンコードが露出しない元の名前で載る。
- 該当の名前のスニペットを `DELETE /api/snippets/bigquery/{name}` (名前は `encodeURIComponent` 済み) で削除でき、204 が返る。Snippets ドロップダウンの削除ボタンから該当の名前を削除できることも手動確認し、結果を issue に記録する。
- 手動で配置したエンコードなしの .sql ファイル (`/` と `%` を含まないファイル名) が従来どおり一覧に載る。
- `%` を含むファイル名 (例: `foo%20bar.sql`) を手動で配置した場合、一覧がエラーにならず、ファイル名がそのまま名前として表示され (`foo%20bar`)、その名前で削除できる。
- `internal/snippet` の単体テストに次が追加される: `/` を含む名前の Save / List / Delete のラウンドトリップ、`%` を含む名前の Save / List / Delete のラウンドトリップ、非正規形のファイル名 (デコード不能な `%` 並び、エンコードなしの `%` を含むファイル名) の一覧と削除、エンコード後のファイル名がバイト長上限を超える名前の拒否。テーブル駆動で athena / bigquery の両サービスキーを対象にし、挙動が同一であることを検証する。
- `handlers_snippets` のテストが更新される: `/` を含む名前の POST が 200 を返すケースの追加と、パス区切り文字を理由とする 400 を期待する既存ケースの新仕様への書き換え (または削除)。
- 扱わない範囲: frontend の変更 (設計判断のとおり不要であることを検証済み)、スニペット保存先ディレクトリの変更 (docs/issues/closed/0143 で決定済み)、frontend の名前入力 UI (`window.prompt`) の変更、`ErrorBanner` の表示文言の変更、Unicode 正規化を行うファイルシステム上での表示名の揺れ、大文字小文字を同一視するファイルシステムでの名前の衝突 (いずれも設計判断の境界条件のとおり)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0143-feat-snippet-default-dir-under-cwd.md: スニペットの保存先ディレクトリを決めた issue。本 issue は保存先は変えず、名前からファイル名への変換だけを変える。

## 解決方法

修正方針のとおり、名前とファイル名の対応にパーセントエンコードを導入した。変更は backend/internal/snippet/snippet.go とテスト 2 ファイルに閉じ、handlers_snippets.go と frontend は変更していない。

- `encodeName`: `/`、`\`、NUL、`%`、先頭のドットを大文字 16 進のパーセントエンコード (`%2F` 形式) に置き換える。それ以外の文字 (全角文字を含む) はエンコードしない。
- `decodeFileName`: `url.PathUnescape` の結果を `encodeName` で再エンコードして元のファイル名に一致する場合 (正規形) だけデコードを適用し、非正規形はファイル名をそのまま名前として扱う。
- `validateName`: 検証を「空でないこと」と「エンコード後のファイル名が 128 バイト以下であること」に縮小し、文字種の制限を撤廃した。
- `Delete`: エンコード済みファイル名を先に探し、無ければ `rawFileNameSafe` (パス区切り・NUL を含まず先頭ドットでない) を条件に名前そのままの後方互換パスを探す。
- `List`: ファイル名の stem を `decodeFileName` で名前に戻す。隠しファイル (先頭ドット) と `.sql` 以外の除外は従来どおり。

修正方針からの乖離 (方式は保ったままの実装詳細。いずれも検討済みの境界条件へ実装時に追記済み):

- エンコード後のバイト長上限の検証は Save だけに適用し、Delete では空チェックのみとした (一覧に出た名前は長さにかかわらず削除できるラウンドトリップを守るため)。
- Delete でファイル名長制限を超える名前 (一覧を経由しない直接リクエストのみ到達可能) は os.Remove の ENAMETOOLONG を ErrNotFound (HTTP 404) に写像した。旧実装の 400 から応答が変わるが 500 にはしない。

完了条件の検証結果:

- 再現手順 2 の curl: 修正後のビルドを `THIEF_LISTEN_ADDR=127.0.0.1:18089` で起動して実施 (ユーザーの 8089 サーバは修正前コードで稼働中だったため。パスの差はリッスンアドレスのみ)。HTTP 200 で `name` が入力と一致し、ディスク上のファイル名は `virtual_money_issue_refund %2F virtual_money_use_refund（取消・返金）.sql` だった。一覧に元の名前で載り、`encodeURIComponent` 済みの DELETE が 204 を返した。
- UI 手動確認 (2026-08-25、ユーザー実施): 8089 のサーバを新コードで再起動し、BigQuery の Snippets でスラッシュ入り名の保存、一覧の元の名前での表示、ドロップダウンの削除ボタンでの削除がすべて成功した。
- 手動配置ファイル: `manual_plain.sql` がそのまま一覧に載り、`foo%20bar.sql` は名前 `foo%20bar` で一覧に載りその名前で削除できることを curl とテスト (`TestStoreListAndDeleteNonCanonicalFileNames`) の両方で確認した。
- 単体テスト: `TestStoreRoundTripUnsafeNames` (`/` `\` NUL `%` トラバーサル 先頭ドットのラウンドトリップ)、`TestStoreListAndDeleteNonCanonicalFileNames`、`TestStoreListDecodesCanonicalFileNames`、`TestStoreListPercentCollision`、`TestStoreDeleteLongName`、`TestStoreDeleteNameTooLongReturnsNotFound`、`TestStoreDeleteDoesNotEscapeServiceDir` を追加し、いずれも athena / bigquery の両サービスで検証した。handlers 側は `TestHandleSnippetSaveNameWithSlash`、`TestHandleSnippetSaveTraversalNameAccepted`、実 ServeMux 経由の `TestSnippetRoutesSlashNameViaMux` を追加し、パス区切りを理由とする 400 の既存ケースは新仕様 (404 またはエンコード後長超過の 400) に書き換えた。
- `mise run check` 通過 (frontend 73 ファイル 742 テスト、backend `go test -race` 全パッケージ ok、govulncheck 0 件)。

多観点レビュー: 初回 5 観点 + 追加 1 ラウンド 5 観点 (実装変更の指摘は初回観点 3 の ENAMETOOLONG 写像 1 件のみで、追加ラウンドはコメント・テスト・ドラフト修正のみ)。却下 1 件 (修正方針 44 行目の「どのファイルでも」の書き換え要求。issue の既存記述は書き換えない運用のため、追記側で例外を明示して決着)。記録のみ 5 件 (制御文字はエンコード対象にしない設計意図、syscall パッケージは新規コードで x/sys 推奨との注記があること、バックスラッシュ入りファイル名テストが Windows で失敗しうること、追記のみであることの機械的検証が issues/ の git 管理外により不能なこと、レビュー依頼文の件数誤記)。レビューで発見した既存バグ (List の列挙と読み取りの間の削除で一覧全体が 500 になる TOCTOU) は本 issue のスコープ外として docs/issues/0152 に登録した。
