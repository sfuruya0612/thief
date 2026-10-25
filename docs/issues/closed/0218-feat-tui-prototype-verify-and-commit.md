# 全画面の TUI (`thief tui`) のプロトタイプを PRD の FR-29 と FR-30 に照らして検証し、PRD と ADR 0033 とともに入れる

Created: 2026-10-25
Model: Claude Fable 5.1
Completed: 2026-10-25

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> 全画面の TUI (`thief tui`) を追加し、EC2 の一覧、Drawer (Overview と Tags)、プロファイル / リージョン / サービスの選択、Session Manager のセッションを端末の中で行えるようにしたい (docs/prd/thief.md の FR-29 と FR-30、docs/adr/0033。2026-10-23 に確認したプロトタイプが `backend/internal/tui/` と `backend/internal/cli/tui.go` に未コミットで置いてあり、これを正規の issue として検証してコミットする)

2026-10-23 時点の作業ツリーには、次の未コミットの変更がある。

- `backend/internal/tui/` (新規): `service.go` の `Service` と `Row` のインターフェース、`ec2.go` の EC2 の実装 (`NewEC2Service`、`ec2Row`)、`model.go` の bubbletea の `Model` (一覧、Drawer、通知、選択肢、対話セッション)、`picker.go` の絞り込み付きの選択肢、`program.go` の `Run`、`styles.go`。テストは `ec2_test.go`、`model_test.go`、`picker_test.go`。
- `backend/internal/cli/tui.go` (新規): `newTUICmd` と `runTUI`、`sessionManagerCommand`。`runTUI` は `tui.Deps` をローカル変数で組み立て、`Services` は `tui.NewEC2Service(awsinternal.ListEC2Resources)` だけを持つ。
- `backend/internal/cli/root.go`: `newTUICmd()` の登録。`backend/internal/cli/run_test.go`: `context.Background()` を使ってよい関数の一覧に `tui.go:sessionManagerCommand` を追加。
- `backend/go.mod` と `backend/go.sum`: `github.com/charmbracelet/bubbles v0.21.0` と `github.com/charmbracelet/lipgloss v1.1.0` を直接依存に追加 (bubbletea は既存の依存)。
- `docs/prd/thief.md` (FR-29、FR-30 と 2026-10-23 の決定)、`docs/adr/0033-tui-in-process-bubbletea.md` (新規)、`docs/adr/README.md`、`AGENTS.md` の `internal/tui/` の節、`CHANGES.md` の [ADD] と `### misc` のエントリ、`docs/issues/TODO.md` の TUI の 3 行。

プロトタイプの動作は 2026-10-23 に開発者が確かめた。
しかし、プロトタイプは PRD の FR-29 の受け入れ基準の次の点をまだ満たしていない。

- 通知の 1 行化: FR-29 は「通知は 1 行で表示し、改行は空白に置き換える」と定めるが、`backend/internal/tui/model.go` の `setStatus` は文字列をそのまま保持し、`View` は `lipgloss.NewStyle().MaxWidth(m.width)` で幅だけを制限する。`sessionExitMsg` の処理で子プロセスの終了エラーと `finalize` の両方が失敗したときの `errors.Join(fmt.Errorf("session process: %w", exitErr), err)` は改行を含み、`bodyHeight` は通知のために 1 行しか引かないため、通知が 2 行以上になるとキー操作のヒントの行が画面の外に押し出される。
- `tui.Deps` の組み立てのテスト: PRD の成功指標「TUI の AWS サービスの網羅」は、`tui.Deps` を組み立てる処理を関数に切り出して `Services` の `ID()` の集合をテストすることを測定方法にしている。`runTUI` はローカル変数で組み立てており、テストから取り出せない。
- `Load` の配線のテスト: FR-29 の受け入れ基準は、`Service` を作る関数に偽の一覧関数を渡して `Load` がそれを呼ぶことを単体テストで確認するとしている。`ec2_test.go` の `TestEC2Service_LoadWrapsErrorAndSetsRegion` は、偽の関数が `profile` と `region` を受け取り結果が行になることを確かめているが、`ec2Service` 構造体を直接組み立てており、`NewEC2Service` を通していない。
- State の表示: FR-29 の受け入れ基準は `Row.State()` と State の列を Web UI の `StatusBadge` が表示する値 (資源の `State` フィールドの生の値) に合わせるが、`backend/internal/tui/service.go` は `Row.State()` を「`aws.NormalizeState` で正規化した状態」と定め、`ec2.go` の `State()` は `ResourceState()` (正規化後の値) を返す。EC2 でも `pending` は正規化で `updating` になり (`ec2_test.go` の `TestEC2Service_LoadWrapsErrorAndSetsRegion` は `State: "pending"` の資源で `State()` が `updating` になることを期待値にしている)、Web UI の `pending` と食い違う。RDS の `available` や ECS の `active` は正規化で `running` になり、18 サービスの追加 (issue 0219 から 0223) がこの契約の上に載ると食い違いが広がる。
- 空の値の表示: `ec2.go` の `Cells()` は Private IP を、`Overview()` は VPC を `orDash` で `—` にするが、Web UI の `ec2Columns` の Private IP と `ec2OverviewRows` の VPC は空のとき空文字列を描画する。`Dash` か `dash` を使うのは列の Public IP と Uptime、Overview の Private IP、Public IP、Uptime、Launched である。
- floci の EC2 対応の記録: FR-29 の受け入れ基準は floci で EC2 インスタンスを 2 台作って一覧を確かめる手順を書くが、`example/README.md` の「確認手順」は S3、DynamoDB、SQS、SSM Parameter Store、Secrets Manager、CloudFormation だけを挙げ、`example/seed.sh` は EC2 インスタンスを作らない。floci の `RunInstances` と `DescribeInstances` が動くことは 2026-10-23 に手元で確かめたが、リポジトリに記録が無い。

## 目的

`thief tui` の第 1 段階の土台 (EC2 の一覧、Drawer、選択肢、通知、EC2 の対話セッション) が、PRD の FR-29 と FR-30 の受け入れ基準のうち EC2 と画面の骨格に関する項目 (完了条件に列挙する) を満たした状態で、PRD、ADR 0033、`AGENTS.md`、`CHANGES.md` とともにリポジトリに入る。
FR-29 の残りの 18 サービスと FR-30 の ECS Exec は後続の issue で扱う。

## 設計判断

- プロトタイプをそのまま検証の対象にし、作り直さない。接続方式 (API サーバを経由しない)、フレームワーク (bubbletea、bubbles、lipgloss)、画面の構成は ADR 0033 と PRD の「決定した論点」で決まっている。作り直す案は、開発者が確かめた体験を捨てることになるので採らない。
- 通知の 1 行化は `setStatus` で改行を空白に置き換える。通知の行数に応じて `bodyHeight` を増やす案は、長い通知が一覧と Drawer を押しつぶし、FR-29 の「通知はヒントの行を隠さない」を満たす代わりに本文を隠すので採らない。
- `tui.Deps` の組み立ては `backend/internal/cli/tui.go` に `newTUIDeps() tui.Deps` のような関数として切り出し、`runTUI` はそれを呼ぶ。`tui` パッケージ側に `Deps` を組み立てる関数を置く案は、`tui` が `internal/aws` と `cli` の `sessionManagerCommand` を知ることになり、取得層の関数を注入する今の形 (`tui` は `aws` の型だけを知る) を崩すので採らない。
- `Load` の配線のテストは、`TestEC2Service_LoadWrapsErrorAndSetsRegion` を `NewEC2Service` 経由に直し、返した資源が行になることと、渡した `profile` と `region` がその関数に届くことを確かめる。18 サービスの追加 (issue 0219 から 0223) は同じ形のテストを各サービスに書く。
- `Row.State()` の契約を「Web UI の `StatusBadge` が表示する値 (資源の `State` フィールドの生の値)」に改め、`service.go` のコメントと `ec2.go` の `State()` (`EC2Resource.State` を返す) を直す。色付け (`styles.go` の `stateStyle`) は `refreshDrawer` が `aws.NormalizeState(row.State())` を掛けてから選ぶ。正規化した値を表示する今の契約を残して `Cells()` だけ生の値にする案は、Drawer の State と列の State が同じ資源で違う文字列になるので採らない。
- `ec2.go` の `Cells()` の Private IP と `Overview()` の VPC は `orDash` を外し、空のとき空文字列にする。空の値を `—` にするのは Web UI の同じセルか項目が `Dash` か `dash` を描画するものに限る、という規則を 18 サービスの issue (0219) が引き継ぐ。
- floci の EC2 対応は `example/README.md` の「確認手順」に EC2 の手順 (`run-instances` 2 台と `thief tui -p floci`) を足して記録する。`example/seed.sh` には EC2 の作成を足さない。FR-29 の受け入れ基準は「`example/seed.sh` は EC2 インスタンスを作らない」ことを前提に、`run-instances` を手で実行してから `/` の絞り込みで `web-01` の 1 行だけになることを確かめる手順になっており、seed に足すとこの前提が崩れる。
- 追加の API 呼び出しと権限は無い。TUI は Web UI と同じ `ListEC2Resources`、`ListProfiles`、`ListRegions`、`StartSSMSession`、`TerminateSSMSession` を使う。
- プロトタイプのコード、PRD、ADR 0033、`AGENTS.md`、`CHANGES.md`、`docs/adr/README.md`、`docs/issues/TODO.md`、`example/README.md` は 1 つのコミットで入れる (PRD の「決定した論点」の「TUI の進め方」)。

## 完了条件

- `backend/internal/tui/model.go` の `setStatus` が改行を空白に置き換え、改行を含む文字列を渡したとき `View()` の通知が 1 行になり、キー操作のヒントの行が最後の行に表示されることを `model_test.go` のテストで確認する。
- `backend/internal/cli/tui.go` に `tui.Deps` を組み立てる関数があり、`runTUI` がそれを使う。その関数の `Services` の `ID()` の集合が `ec2` だけであることをテストで確認する (18 サービスの追加で、このテストの期待値を PRD の 19 個に広げる)。
- `backend/internal/tui/ec2_test.go` に、`NewEC2Service` に偽の一覧関数を渡して `Load` を呼ぶと、渡した `profile` と `region` でその関数が呼ばれ、返した `aws.EC2Resource` が行になることを確かめるテストがある (`ec2Service` を直接組み立てる形ではない)。
- `backend/internal/tui/service.go` の `Row.State()` のコメントが、Web UI の `StatusBadge` が表示する値 (資源の `State` フィールドの生の値) を返すと定め、`ec2.go` の `State()` が `EC2Resource.State` を返し、`model.go` の Drawer の State の色が `aws.NormalizeState(row.State())` で選ばれる。`ec2_test.go` で、`State: "pending"` の資源の `State()` が `pending` になることを確認する (`TestEC2Service_LoadWrapsErrorAndSetsRegion` の既存の期待値 `updating` を `pending` に更新する)。`model_test.go` に、`State()` が `available` を返す行を選んだとき Drawer の State の文字列が `available` であることを確かめるテストがある。
- `ec2.go` の `Cells()` の Private IP と `Overview()` の VPC が、空のとき空文字列になる (`—` にしない) ことを `ec2_test.go` で確認する。列の Public IP と Uptime、Overview の Private IP、Public IP、Uptime、Launched は引き続き `—` になる。
- PRD の FR-29 の受け入れ基準のうち、次をこの issue の「## 動作確認」に結果とともに記録する: `thief tui --help`、floci の 2 台の一覧と Drawer の Overview と Tags、`/` の絞り込み、`p` の選択肢の絞り込み `(1/1)`、`mise run example:down` の後の `r` の失敗表示と `example:up` の後の `r`、`mise run example:down` の後に `R` を押して出る通知が 6 秒後に消えること、`q` と画面の Ctrl-C と `kill -TERM` の終了コード 0、`kill -INT` の終了コード 130、`lsof` の待ち受け無し、`go list -deps ./internal/tui/` に `internal/api` が無いこと。
- PRD の FR-29 の受け入れ基準のうち、実 AWS が要るもの (認証情報が無効なプロファイルでの `p` と `R`、SSO の期限切れのバナー) と FR-30 の受け入れ基準 (EC2 のセッションの `echo ok`、`session closed`、`describe-sessions`、Ctrl-C の後の `exit` と `r`、`session-manager-plugin` が無いときの通知) は、開発者が実 AWS で確認して結果を「## 動作確認」に記録する。確認できなかった項目は、その旨と理由を書く。
- `example/README.md` の「確認手順」に、floci で EC2 インスタンスを 2 台作って `HOME="$(pwd)/example/home" thief tui -p floci` で一覧を確認する手順がある。
- PRD の次の 3 か所を、この issue で済ませた事実に合わせて書き換える: 成功指標「TUI の AWS サービスの網羅」の測定方法の「2026-10-23 時点の `runTUI` はローカル変数で組み立てており、切り出しはプロトタイプの issue で行う」、FR-29 の通知の「2026-10-23 時点のプロトタイプは改行をそのまま描画しており、プロトタイプの issue で直す」、FR-29 の受け入れ基準の floci の EC2 の「リポジトリには記録が無く、`example/README.md` への記録はプロトタイプの issue で行う」。
- `CHANGES.md` の `## develop` の TUI の [ADD] のエントリ 1 つに、通知の 1 行化と `Row.State()` の生の値への変更 (EC2 の `pending` の表示が `updating` から `pending` に変わる) が含まれている。`docs/adr/README.md` に 0033 の行がある。`AGENTS.md` の `internal/tui/` の節と ADR 0033 に、通知を複数行で描画する、`runTUI` がローカル変数で `Deps` を組み立てる、`Row.State()` が `aws.NormalizeState` で正規化した値を返す、のいずれかを述べる文が無い (あれば、この issue の実装に合わせて書き換える)。
- `mise run check` が通る。
- 扱わない範囲: EC2 以外の 18 サービス (issue 0219 から 0223)、ECS Exec (issue 0224)、PRD の「未確定論点」の項目。

## 関連

- docs/prd/thief.md の FR-29、FR-30、「決定した論点」の 2026-10-23 の表
- docs/adr/0033-tui-in-process-bubbletea.md
- issue 0219 から 0223 (18 サービスの一覧と Drawer)、issue 0224 (ECS Exec)。この issue が入れる `Service` と `Row` のインターフェース、`Deps` の組み立ての関数、`Load` の配線のテストの形を、それらの issue が使う。

## 動作確認

2026-10-25 に floci で確認した。`thief` は `mise run backend:install` で導入した。floci は `docker compose -f example/compose.yaml up -d` で起動した (`mise run example:up` と同じ compose ファイル。`example:up` は前面で動き続けるため `-d` を付けた)。EC2 インスタンスは `example/README.md` の「TUI (`thief tui`) の EC2 一覧」の `run-instances` で `web-01` と `batch-01` の 2 台を作った。

TUI の操作は Python の `pty.fork` で `HOME="$(pwd)/example/home" thief tui -p floci` を起動する駆動スクリプトで行った (`~/.claude/tmp/claude-2013771024/-Users-shoichi-furuya-apps-thief/fa65ebe8-98bd-4ac0-b99c-e41e7390dfc1/scratchpad/tui_smoke.py`。結果は同じ場所の `tui_smoke.out` で、22 項目すべて OK)。スクリプトは次の 3 点を扱う。端末の背景色の問い合わせ (OSC 11) に応答する (応答が無いと起動が 5 秒止まる)。キーは 1 文字ずつ送る (まとめて送ると bubbletea が `1/` のような複数文字を 1 つの KeyMsg にし、単独キーの判定に掛からない)。画面はウィンドウの幅を 1 桁変えて全面を描き直させ、ANSI シーケンスを取り除いて読む。

### floci で確認した項目

| 項目 | 結果 |
| --- | --- |
| `thief tui --help` | 終了コード 0。次の出力が出る |
| 2 台の一覧 | タイトルが `EC2 (2)`、列が `Name State Type Region AZ Private IP Public IP Uptime`、行が `web-01 running t3.micro ap-northeast-1 b 172.31.16.10 — 0h 0m` と `batch-01 running t3.micro ap-northeast-1 b 172.31.16.11 — 0h 0m` |
| Drawer の Overview | `Name web-01`、`State running`、`Resource ID i-f9181993c6a93adb4`、`Type t3.micro`、`AZ ap-northeast-1b`、`Region ap-northeast-1`、`Private IP 172.31.16.10`、`Public IP —`、`VPC`、`Uptime`、`Launched` の順に出る (Resource ID は実行ごとに変わる) |
| Drawer の Tags (`2`) | `Name  web-01` の 1 行。`1` で Overview に戻る |
| `/` で `web` | 一覧が `web-01` の 1 行になり、タイトルが `/ web` になる。Enter で絞り込みを保ったままタイトルが `EC2 (1/2)  / web` になる。`/` の後の esc で入力が消え `EC2 (2)` と 2 行に戻る |
| `p` で `floci` | `p` を押すと `Select a profile (1/1)` と選択肢 `floci` の 1 件が出る。`floci` と入力しても `(1/1)` と `floci` のまま (`example/home/.aws/config` のプロファイルは `floci` の 1 件だけなので、入力で件数が減ることは floci では確かめられない。入力で件数が減ることは `model_test.go` の `TestModel_PickerFiltersByTyping` で確認している)。esc で入力が消え `> type to filter` に戻り、もう 1 度 esc で閉じて一覧に戻る |
| `lsof -nP -p <pid> -a -i -sTCP:LISTEN` | 出力なし |
| `mise run example:down` の後の `r` | 取得中の表示の後、一覧の領域に `list EC2 instances: describe ec2 instances: operation error EC2: DescribeInstances, exceeded maximum number of attempts, 3, https response error StatusCode: 0, RequestID: , request send failed, Post "http://localhost:4566/": dial tcp [::1]:4566: connect: connection refused` が出る |
| `example:up` の後の `r` | 失敗の表示が消え、`EC2 (0)` と `No EC2 resources found` になる (コンテナを作り直すとインスタンスは消える)。2 台を作り直して `r` を押すと 2 行に戻る |
| `mise run example:down` の後の `R` | 通知行に `loading regions...` が出て、約 5.5 秒後に `list regions: describe regions: operation error EC2: DescribeRegions, exceeded maximum number of attempts, 3, https response error StatusCode: 0, ...` に変わる。どちらの間もキー操作のヒントの行 (`/ filter  j/k move ... q quit`) は最後の行に表示されたまま。キーを押さずに 6 秒待つと通知行が消えている |
| `q`、画面の Ctrl-C、`kill -TERM <pid>` | 終了コード 0 |
| `kill -INT <pid>` | 終了コード 130 |
| `cd backend && go list -deps ./internal/tui/` | `internal/api` を含まない |

`thief tui --help` の出力。

```
Opens a full-screen terminal UI that lists AWS resources, shows their details,
and starts interactive sessions, like the web UI but inside the terminal.

Usage:
  thief tui [flags]

Flags:
  -h, --help   help for tui

Global Flags:
  -g, --group-by string   Group output by column name(s) and show count (comma-separated for multiple)
      --no-header         Hide the header in output
  -o, --output string     Output format (tab/csv)
  -p, --profile string    AWS profile (default uses environment or config file)
  -r, --region string     AWS region (default ap-northeast-1)
```

### 実 AWS が要る項目 (未確認)

次の項目は確認していない。この issue の実装は開発者の離席中の自律実行で行い、実 AWS のプロファイル (認証情報が無効なプロファイルと SSO のプロファイル) と、Session Manager で接続できる EC2 インスタンスを使わなかったためである。開発者が実 AWS で確認し、結果をこの節に追記する。

- FR-29: 認証情報が無効なプロファイルに `p` で切り替えたときの `list EC2 instances:` の失敗表示と、続けて `R` を押したときの `list regions:` の通知 (floci を止めた状態の `r` と `R` で、同じ経路の失敗表示は確認した)
- FR-29: SSO の期限切れのバナー (`thief sso login` の案内)
- FR-30: EC2 のセッションでの `echo ok`、終了後の `session closed` の通知、`aws ssm describe-sessions` でのセッションの切断、Ctrl-C の後の `exit` と `r`、`session-manager-plugin` が無いときの通知


## 解決方法

プロトタイプを作り直さず、完了条件に挙げた差分だけを直してコミットした。

- 通知の 1 行化: `backend/internal/tui/model.go` の `setStatus` が `statusLineReplacer` (`strings.NewReplacer`) で CRLF、CR、LF を空白に置き換える。`model_test.go` の `TestModel_StatusWithNewlinesRendersAsOneLine` は、`finalize` が失敗する `sessionReadyMsg` の後に終了エラー付きの `sessionExitMsg` を渡し (2 つの失敗は `case sessionExitMsg` が `errors.Join` でまとめる)、通知に改行が無いこと、`View()` の最後の行がキー操作のヒントであること、2 つの失敗が 1 行に入ることを確かめる。`TestModel_StatusReplacesCRLFAndCR` は CRLF、CR、LF、混在のそれぞれが 1 つの空白になることを確かめる。
- `tui.Deps` の組み立て: `backend/internal/cli/tui.go` に `newTUIDeps() tui.Deps` を切り出し、`runTUI` は `tui.Run(newTUIDeps(), cfg.Profile, cfg.Region)` を呼ぶ。`backend/internal/cli/tui_test.go` の `TestNewTUIDeps_ServicesAreEC2Only` は、`Services` の `ID()` の集合が `ec2` の 1 個で重複が無いこと、`ListProfiles`、`ListRegions`、`StartSession` が nil でないことを確かめる。
- `Load` の配線のテスト: `backend/internal/tui/ec2_test.go` の `TestEC2Service_LoadWrapsErrorAndSetsRegion` を `NewEC2Service` に偽の一覧関数を渡す形に直し、呼ばれた `profile/region` の記録が `p/us-west-2` の 1 件であること、偽の一覧関数が返した `aws.EC2Resource` が行になることを確かめる。
- State の表示: `backend/internal/tui/service.go` の `Row.State()` のコメントを「Web UI の `StatusBadge` が表示する値 (資源の `State` フィールドの生の値)」に改め、`ec2.go` の `State()` は `EC2Resource.State` を返す。`model.go` の `refreshDrawer` は `stateStyle(aws.NormalizeState(row.State())).Render(row.State())` で、色の選択にだけ正規化した値を使う。`TestEC2Service_LoadWrapsErrorAndSetsRegion` の期待値を `pending` に変え、`model_test.go` の `TestModel_DrawerShowsRawState` は `State()` が `available` の行を選んだとき Drawer に `available` が出ることを確かめる。
- 空の値の表示: `ec2.go` の `Cells()` の Private IP と `Overview()` の VPC から `orDash` を外した。`ec2_test.go` の `TestEC2Row_EmptyValuesFollowWebUI` は、空の資源の列が `web, stopped, "", ap-northeast-1, "", "", —, —` であること、Overview の Private IP、Public IP、Uptime、Launched が `—` で VPC が空文字列であることを確かめる。
- floci の記録: `example/README.md` の「確認手順」に「TUI (`thief tui`) の EC2 一覧」を足し、`run-instances` で `web-01` と `batch-01` を作り、`HOME="$(pwd)/example/home" thief tui -p floci` で `/` の絞り込みと Drawer を確かめる手順を書いた。
- PRD: 成功指標「TUI の AWS サービスの網羅」の測定方法を `newTUIDeps` と `backend/internal/cli/tui_test.go` (2026-10-25 時点の期待値は `ec2` の 1 個) に書き換え、FR-29 の通知の「プロトタイプは改行をそのまま描画しており」の括弧書きを削除し、FR-29 の floci の EC2 の括弧書きを `example/README.md` の「確認手順」への参照に書き換えた。
- `CHANGES.md` と関連文書: TUI の [ADD] のエントリに、通知の 1 行化 (改行は空白に置き換える) と、一覧と Drawer の State が資源の `State` の生の値になること (EC2 の `pending` は `updating` ではなく `pending` と出る) を足した。`docs/adr/README.md` の 0033 の行、`AGENTS.md` の `internal/tui/` の節、ADR 0033 に書き換えが要る文は無かった。
- 動作確認: 「## 動作確認」に記録した。floci で確かめた 22 項目はすべて期待どおりで、実 AWS が要る項目は未確認として理由を書いた。
- `mise run check`: `mise run backend:vuln` (govulncheck) は、Go 1.26.6 の標準ライブラリ (`crypto/x509` の GO-2026-6809 は Go 1.26.10、`net/http`、`crypto/tls`、`mime/multipart`、`html/template` の 9 件は Go 1.26.9 で修正) と `golang.org/x/net` v0.59.0 (GO-2026-6617 など、v0.60.0 で修正) の 12 件で失敗する。最初の検査では 2 件、レビューの反映後の再検査では 12 件で、増加は govulncheck が実行のたびに取得する脆弱性データベースの更新による。いずれもこの issue の差分 (bubbles と lipgloss の追加) が触らない既存の依存であり、Go のツールチェインと `x/net` のバージョンは HEAD と同じなので、既知の失敗として扱い、ツールチェインと `x/net` の更新を `docs/issues/TODO.md` に積んだ。それ以外 (gofmt、goimports、`backend:lint`、`backend:test`、prettier、`frontend:lint`、`frontend:test`) は通る。
- レビューの反映: 多観点レビュー (完了条件、テスト、規約の 3 観点) の指摘で次を直した。SSO の期限切れの判定を Web UI の `writeAWSError` と同じ順にし、`aws.IsAccessDenied` で IAM の権限不足を除いてから `aws.IsSSOTokenExpired` を見る (`TestModel_AccessDeniedDoesNotRaiseSSOBanner`)。プロファイルとリージョンの切替でもサービスの切替と同じく旧い行を消してから取り直す `reloadForNewTarget` にまとめ、取得中に `s` を押しても旧い行でセッションを始めない (`TestModel_ProfileSwitchClearsRowsUntilReload`、`TestModel_RegionSwitchClearsRowsUntilReload`、`r` は行を残す `TestModel_ReloadKeepsRowsWhileLoading`)。`sessionStarting` を `startSession` で立て `sessionReadyMsg` で下ろし、開始中と `inSession` の間は `s` を無視する (`TestModel_SessionKeyIgnoredWhileStarting`、`TestModel_SessionKeyIgnoredDuringSession`)。`Init` の重複した `spinner.Tick` を除く (`TestModel_InitTicksSpinnerOnce`)。プロファイルの一覧が届く前の `p` は空の選択肢を開かず、`profilePickerPending` を立てて取り直し、届いたら開く (`TestModel_ProfileKeyBeforeProfilesLoadedWaitsForList`、失敗は `TestModel_ProfileKeyBeforeProfilesLoadedReportsFailure`)。`backend/internal/cli/tui.go` の `sessionManagerCommand` を `tuiSessionDeps` を受ける `sessionManagerCommandWith` に分け、`tui_test.go` で成功時の引数と `finalize` の切断、開始の失敗、plugin が無いときの切断、切断の失敗の両方の報告を確かめる (`run_test.go` の `context.Background` の許可一覧は `sessionManagerCommandWith` に改名)。`TestModel_DrawerShowsRawState` は `View()` 全体ではなく Drawer の `State` の行を見る。幅 40 で 200 文字の通知が 1 行に収まりヒントが最後の行に残ることを `TestModel_LongStatusFitsNarrowWidth` で確かめる。`SupportsSession` が偽のサービスの `s` と `profilesLoadedMsg` の失敗のテストを足した。`ec2.go` と `ec2_test.go` の dash の規則のコメント、`example/README.md` の `mise run example:up` の説明 (前面で動き続ける)、`CHANGES.md` の State の色の文を直した。
