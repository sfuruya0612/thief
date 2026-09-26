# スニペットのデフォルト保存先をサーバ起動時のカレントディレクトリ配下に変更する

Created: 2026-08-15
Model: Claude Fable 5
Completed: 2026-08-15

## 背景

docs/issues/TODO.md の次の項目に対応する。

> BigQuery, Athena, CloudLogging のスニペット保存先が /tmp/ になってるせいで永続化できてない。
> サーバが起動しているディレクトリ配下をデフォルトの保存先としたい

クエリスニペットは `backend/internal/snippet/snippet.go` の `Store` がサービス別ディレクトリに `.sql` ファイルとして保存する。保存先は `api.NewServer` (`backend/internal/api/server.go` 64 行目) が渡す `cfg.SnippetsDir` で決まり、そのデフォルト値は `backend/internal/config/config.go` の `Defaults()` が返す `/tmp/thief` である。`/tmp` 配下は OS の再起動や定期削除で消えるため、保存したスニペットが永続化されない。

上書き手段は 2 つある。環境変数 `THIEF_SNIPPETS_DIR` (`applyEnv`) と、設定ファイルの `snippets-dir` キー (`applyFile`) である。問題はデフォルト値だけであり、上書きの仕組みは変更しない。

TODO は BigQuery、Athena、CloudLogging の 3 サービスを挙げているが、サーバ保存のスニペット機能を持つのは athena と bigquery の 2 つだけである (`snippet.go` の `services` マップと、`frontend/src/lib/queryEditorStorage.ts` の `QueryEditorService` 型のいずれも 2 サービスのみを定義している)。`frontend/src/views/nonaws/CloudLoggingView.tsx` にある「スニペット」は静的なプリセットフィルタの配列で、`/api/snippets` を呼んでいない。TODO が CloudLogging を挙げた背景の特定は本 issue では扱わない。本 issue の対象は athena と bigquery であり、CloudLogging へのスニペット保存機能の追加は扱わない。CloudLogging にサーバ保存スニペットが必要になった場合は、本 issue とは別に起票する。

## 目的

OS の再起動や定期削除を経ても保存済みスニペットが失われない状態にする。デフォルトの保存先を、サーバを起動したディレクトリ (プロセスのカレントディレクトリ。以下 CWD) 配下の `.thief/snippets` に変更する。

## 設計判断

- デフォルト値は CWD 相対の固定文字列 `.thief/snippets` (`filepath.Join(".thief", "snippets")`) とする。サーバは起動後に CWD を変更しない (`os.Getwd` も `os.Chdir` も backend に出現しない) ため、相対パスのままで一貫して同じ場所を指す。
  - 採らなかった案: `config.Load()` 時に `os.Getwd()` で絶対パス化する。サーバは CWD を変更しないため絶対パス化しても指す先は変わらず、得られる利益は保存失敗時のエラーメッセージ (`Store.Save` の `create snippets dir %s: %w` 等) に絶対パスが表示される読みやすさだけである。この利益は薄いと判断して却下。
- ディレクトリ名は `.thief/snippets` とする。`thief` の名前空間で他のファイルとの衝突を避け、隠しディレクトリにすることでオプションなしの `ls` の一覧に現れないようにする。git 管理上の汚れは後述の `.gitignore` への追加で防ぐ。
  - 採らなかった案 1: `config.Dir()` 配下 (`$XDG_CONFIG_HOME/thief` または `~/.config/thief`) に置く。永続状態の置き場所として一般的であり、gcp プロジェクト一覧キャッシュの置き場所とも揃うが、TODO が「サーバが起動しているディレクトリ配下」と明示しているため却下。
  - 採らなかった案 2: CWD 直下の `snippets/`。ユーザーの既存ディレクトリと衝突しうるため却下。
- CWD 相対のデフォルトは、`go install` 済みの `thief server` をどのディレクトリから起動するかで保存先が変わる。これは TODO の要望自体が持つ性質であり、固定の保存先が必要な運用では `THIEF_SNIPPETS_DIR` か `snippets-dir` で絶対パスを指定する。
- 起動時に保存先の書き込み可否は検査しない。CWD が書き込み不可 (読み取り専用ディレクトリ等) の場合、サーバ起動は成功し、保存操作の時点で `Store.Save` 内の `os.MkdirAll` または `os.CreateTemp` が失敗して `POST /api/snippets/{service}` が `writeSnippetError` 経由の 500 (`SNIPPET_ERROR`) を返す。失敗はこの API エラーとしてユーザーに見える。
  - 採らなかった案: 起動時に `os.MkdirAll` で保存先を作成し、失敗したら警告ログを出す。スニペットを使わない起動でも CWD に `.thief/` を作る副作用が生じるため却下。
- `SnippetsDir` は API サーバ専用フィールドで、CLI サブコマンド (`backend/internal/cli/helper.go` の `loadConfig()`) でも `config.Load()` 経由で CWD 相対のデフォルト値が `Config` に入るが、CLI はこのフィールドを参照しないため挙動は変わらない。
- `/tmp/thief` からの自動移行は行わない。`/tmp` のデータは再起動で消える前提のものであり (それが本 issue の動機)、まだ残っている環境では `THIEF_SNIPPETS_DIR=/tmp/thief` で旧位置を指し続けられる。
  - 採らなかった案 1: フォールバック探索 (新位置に無ければ旧位置を読む)。Save は新位置、List と Delete は旧位置のファイルを対象にでき、同名スニペットの所在によって挙動が変わるため却下。
  - 採らなかった案 2: 旧ディレクトリにファイルが残っていた場合の起動時警告ログ。`/tmp/thief` が残存するのは次回の OS 再起動までで、警告が届く期間と対象がごく限られるため却下。
- `PriceCacheDir` のデフォルト (`/tmp/thief/price`) は変更しない。価格キャッシュは API から再取得できる派生データであり、揮発してよい。
- `mise run backend:run` は CWD が `backend/` になる (`mise.toml` の `dir = "backend"`) ため、このサーバに対してスニペットを保存するとリポジトリ内に `backend/.thief/` が生成される (起動しただけでは生成されない)。ルートの `.gitignore` に `.thief/` を追加して追跡対象外にする。パターンは既存の `/backend/thief` のようなルート限定形ではなく、先頭 `/` なしの `.thief/` とし、CWD に応じてどの階層に生成されても一致させる。
- 追加の API 呼び出しや権限は不要。

## 完了条件

- `config.Defaults()` の `SnippetsDir` が `.thief/snippets` を返す (次項の `TestDefaultsSnippetsDir` で検証する)。
- `backend/internal/config/config_test.go` に `TestDefaultsSnippetsDir` / `TestApplyFileSnippetsDir` / `TestApplyEnvSnippetsDir` を追加する。それぞれ既存の `TestDefaultsPriceCacheDir` / `TestApplyFilePriceCacheDir` / `TestApplyEnvPriceCacheDir` と同型で、デフォルト値 `.thief/snippets`、`snippets-dir` キーによる上書き、`THIEF_SNIPPETS_DIR` による上書きを検証する。
- ルートの `.gitignore` に `.thief/` (先頭 `/` なし) を追加する。
- 手動確認の前提: 次の 2 つの手動確認は、CWD に `config.yaml` が無く、`$XDG_CONFIG_HOME/thief/config.yaml`、`~/.config/thief/config.yaml`、`~/.thief/config.yaml` のいずれも `snippets-dir` を指定しておらず、環境変数 `THIEF_SNIPPETS_DIR` を設定していない環境で実施する。実施前に `env | grep THIEF_` の出力が空であることと、`ls` で上記 4 パスの `config.yaml` の有無 (存在する場合は `snippets-dir` キーの不在) を確認する。どちらの確認も `mise run check` では検証されない。
- 手動確認 1: 事前に `mise run backend:install` で `thief` を導入し、リポジトリ外のディレクトリに移動して `pwd` で絶対パスを控えてから `thief server` を起動する。別ターミナルから `curl -X POST http://127.0.0.1:8089/api/snippets/athena -H 'Content-Type: application/json' -d '{"name":"t","sql":"select 1"}'` と、パスの `athena` を `bigquery` に替えた同じリクエストを実行し、起動時の CWD 配下に `.thief/snippets/athena/t.sql` と `.thief/snippets/bigquery/t.sql` が作られていることを確認する。確認後、このサーバを終了する (終了しないと次の確認のサーバと listen アドレス `127.0.0.1:8089` が競合する)。
- 手動確認 2: 手動確認 1 のサーバを終了した後、`mise run backend:run` で起動したサーバに対して別ターミナルから athena への curl 1 件だけを再実行し、`backend/.thief/snippets/athena/t.sql` が存在することを確認したうえで、`git status` の出力に `backend/.thief/` が現れないことを確認する。この確認の目的は `.gitignore` の実効確認であり、保存経路は手動確認 1 で検証済みのため athena 1 件で足りる。
- `PriceCacheDir` のデフォルト値は `/tmp/thief/price` のまま変更しない。
- CloudLogging へのサーバ保存スニペット機能は追加しない。
- `mise run check` が通る。

## 解決方法

`backend/internal/config/config.go` の `Defaults()` で `SnippetsDir` の既定値を `"/tmp/thief"` から `filepath.Join(".thief", "snippets")` (サーバ起動時のカレントディレクトリ相対) に変更した。`applyFile` / `applyEnv` による `snippets-dir` / `THIEF_SNIPPETS_DIR` の上書きロジックは変更していない (既存の `PriceCacheDir` と同型のパターンを維持している)。

ルートの `.gitignore` に `.thief/` を追加した (先頭 `/` を付けず、CWD に応じてどの階層に生成されても一致させる)。

`backend/internal/config/config_test.go` に `TestDefaultsSnippetsDir` / `TestApplyFileSnippetsDir` / `TestApplyEnvSnippetsDir` を新規追加し、既定値、`snippets-dir` キーによる上書き、`THIEF_SNIPPETS_DIR` による上書きの 3 経路を検証する (既存の `PriceCacheDir` 系テストと同型)。

完了条件の手動確認は次のとおり実施した。実施前に、`env | grep THIEF_` の出力が空であること、および `$XDG_CONFIG_HOME/thief/config.yaml`、`~/.config/thief/config.yaml`、`~/.thief/config.yaml`、カレントディレクトリの `config.yaml` のいずれも存在しないことを確認した。

- 手動確認 1: `mise run backend:install` で `thief` を導入し、リポジトリ外のスクラッチディレクトリで `thief server` を起動したうえで、athena と bigquery のスニペット作成 API を curl で呼び出した。起動時の CWD 配下に `.thief/snippets/athena/t.sql` と `.thief/snippets/bigquery/t.sql` が生成されることを確認し、サーバを終了した。
- 手動確認 2: `mise run backend:run` (CWD が `backend/`) でサーバを起動し、athena への curl を実行した。`backend/.thief/snippets/athena/t.sql` が生成されていることと、`git status` の出力に `backend/.thief/` が現れないこと (`.gitignore` の実効確認) を確認し、サーバを終了した。

検証で生成した `.thief/` 配下のファイルはいずれも削除済み。

`mise run check` を実行し、ベースライン (govulncheck の stdlib CVE によるツールチェイン起因の失敗。issue 0144 として別途登録済み) からの新たな失敗が無いことを確認した。

多観点レビューで次を反映した。

- テストの期待値 `.thief/snippets` を Unix パス区切りのハードコード文字列ではなく `filepath.Join(".thief", "snippets")` に揃え、Windows でも実装の組み立て方と一致した検証になるようにした。
- `CHANGES.md` のエントリ種別をドラフト段階の `[UPDATE]` から `[CHANGE]` に変更した。デフォルト値のみに依存しているユーザーは、明示設定なしに起動すると従来 `/tmp/thief` 配下にあったスニペットが新しい保存先からは見えなくなり、後方互換のある変更とは言えないため。

却下した指摘は次のとおり。

- `TestApplyEnvSnippetsDir` に「環境変数未設定時はデフォルト維持」のケースを追加すべきという指摘。既存の `TestApplyEnvPriceCacheDir` が同型のケースを持たない前例に倣い、今回は追加しないこととした。
- `docs/issues/TODO.md` 92 行目の項目文言に残る `CloudLogging` が実際の対応範囲 (athena / bigquery のみ) と食い違っているという指摘。`docs/issues/TODO.md` の該当行はチェックボックスと参照の書き換えのみが本スキルの担当範囲であり、文言自体の修正は対象外のため対応しない。
