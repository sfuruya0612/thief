# 0034 example の floci 用ダミー HOME に生成されるツールキャッシュを gitignore する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8 claude-opus-4-8

## 背景 / 根拠

issue 0031 で example (floci ローカル動作確認環境) を standalone 構成に作り替えた際、backend をネイティブ起動するときに `HOME` を `example/home` へ差し替えて実 `~/.aws` から隔離する方式にした。
これは thief の `ListProfiles` (`backend/internal/aws/profiles.go`) が `os.UserHomeDir()` を直接参照し `AWS_CONFIG_FILE` 等の環境変数では隔離できないためである。

しかし `HOME` を `example/home` に向けたまま `mise run backend:run` (内部で `go install` を実行) を行うと、Go のビルドキャッシュとテレメトリが `example/home/Library/Caches/go-build/` や `example/home/Library/Application Support/go/` に書き込まれる。
これらはリポジトリ配下に生成されるため `git status` に大量の untracked ファイルとして現れ、誤って `git add` される恐れがある。

issue 0033 の実機検証中にこの生成物を確認した。
example ワークフローを使うたびに再発するため、gitignore で恒久的に除外する。

## 対応内容

- `.gitignore` に、`example/home` 配下のうち追跡対象 (`.aws` のダミー設定) を除くすべてを無視する規則を追加する。
- 具体的には `example/home/*` を無視し、`!example/home/.aws` で `.aws` (config / credentials) のみ追跡対象として残す。

## スコープ外

- example ワークフロー自体の変更 (HOME 差し替え方式は issue 0031 で確定済み)。

## 検証

- `example/home` 配下に Go キャッシュ (`Library/`) を生成した状態で `git status` に現れないことを確認する。
- `example/home/.aws/config` と `example/home/.aws/credentials` は引き続き追跡対象であることを確認する。

## 解決方法

`.gitignore` に以下を追加した。

```
/example/home/*
!/example/home/.aws
```

`example/home/Library/Caches/go-build/` にダミーファイルを作った状態で `git status --short` に何も現れないこと、`git check-ignore` が当該ファイルを `/example/home/*` 規則で無視すること、`git ls-files example/home/.aws/` が `config` / `credentials` を引き続き列挙することを確認した。
