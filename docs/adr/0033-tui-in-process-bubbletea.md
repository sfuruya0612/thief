# 0033. TUI は bubbletea で作り、取得層をプロセス内で直接呼ぶ

Created: 2026-10-23
Model: Claude Fable 5.1
Status: Accepted
Decided: 2026-10-23

## 状況

thief は Web UI (React の SPA) と CLI を持つ (ADR 0002、ADR 0003)。
Web UI を使うには、利用者が `thief server` で API サーバを起動し、別途ブラウザで frontend を開く必要がある。
CLI の一覧コマンド (`thief ec2 ls` など) は 1 回の出力で終わり、プロファイル、リージョン、サービスを切り替えながら一覧と詳細を見比べる操作には向かない。
端末の中だけで Web UI に相当する一覧、詳細、対話セッションを行う手段が無かった。

開発者は 2026-10-23 に、全画面の TUI (`thief tui`) を追加すると決めた (PRD FR-29、FR-30)。
決める必要があったのは、TUI が backend のデータをどう得るかと、どのフレームワークで画面を作るかである。

- データの得方の候補は、API サーバを HTTP で呼ぶ方式と、`backend/internal/aws` などの取得層をプロセス内で直接呼ぶ方式の 2 つであった。
- フレームワークの候補は、`github.com/charmbracelet/bubbletea` (CLI の対話の選択を行う `backend/internal/util/selecter.go` の `Select` が既に直接依存している) と、`github.com/rivo/tview` の 2 つであった。

## 決定

- TUI は API サーバを経由せず、CLI と同じ取得層 (`backend/internal/aws` など) をプロセス内で直接呼ぶ。
  TUI はネットワークの待ち受けを持たず、`thief server` を起動しない。
  取得層への依存は `backend/internal/tui/service.go` の `Service` と `Row` のインターフェースに限り、サービスごとの実装 (`backend/internal/tui/<service>.go`) が取得層の関数を受け取る。
  サービスの一覧は `backend/internal/cli/tui.go` の `Deps.Services` で組み立てる。
- 画面は `github.com/charmbracelet/bubbletea` で作り、部品に `github.com/charmbracelet/bubbles` (table、textinput、viewport、spinner) と `github.com/charmbracelet/lipgloss` を使う。
  `bubbles` と `lipgloss` を直接依存に加える。
  他の TUI フレームワークは導入しない。
- TUI はリソースの一覧の取得結果をキャッシュしない。
  プロファイル、リージョン、サービスを切り替えるたびに取得層を呼ぶ。
  プロファイルの一覧は起動時に 1 回取得する。
- EC2 と ECS への対話セッションは、CLI と同じく PATH にある `session-manager-plugin` に端末を渡して行う (ADR 0012 の CLI の決定を引き継ぐ)。
  bubbletea の `tea.ExecProcess` でプラグインを起動し、終了後に `TerminateSSMSession` でセッションを終える。
- `backend/internal/tui/program.go` の `Run` は、cobra から渡されるシグナル連動の context (ADR 0023) を受け取らず、`context.Background()` から自前の context を作る。
  対話セッション中は bubbletea が端末を通常モードに戻すため、Ctrl-C が SIGINT としてプロセスグループに届き、cobra の context を取り消す。
  その context を取得層に渡すと、セッションから戻った後のすべての取得が失敗する。
  TUI の画面では Ctrl-C は raw モードのキー入力として届き、`q` と同じく終了として扱う。
  SIGTERM は bubbletea 自身が `QuitMsg` に変換する。
  これらの終了コードは 0 であり、CLI の中断の終了コード 130 (ADR 0023) は適用しない。
  プロセスの外から送られた SIGINT (`kill -INT`) は bubbletea が `InterruptMsg` に変換して `ErrInterrupted` を返し、`run.go` が cobra の context の取り消しを見て CLI と同じく 130 で終わる。
  `TestNoRootContextOutsideDesignatedFunctions` (`backend/internal/cli/run_test.go`) の許可リストに `tui.go` の `sessionManagerCommand` を加え、`context.Background()` の使用をテストで固定する。

## 検討した代替案

- API サーバを HTTP で呼ぶ。
  利用者が `thief server` を別に起動する必要があり、「端末の中だけで完結する」という TUI の目的に反する。
  TUI のプロセスの中で API サーバを起動して自分で呼ぶ形も、Origin と Host の検査 (ADR 0032) と JSON の往復が増えるだけで、得られるものが無い。
  取得層は CLI が既にプロセス内で呼んでおり、TUI も同じ形で呼べる。
- `github.com/rivo/tview` を使う。
  tview は tcell の上に部品 (テーブル、フォーム、ページ) を揃えており、画面を速く組める。
  ただし bubbletea は `util.Select` で既に直接依存に含まれており、tview を加えると TUI フレームワークが 2 つになる。
  依存を最小に保つ方針 (ADR 0004) に反するため採らなかった。
- Web UI の機能を TUI に置き換え、ブラウザのクライアントを廃止する。
  Web UI にだけある機能 (分割表示、グラフ、オブジェクトのプレビュー) を TUI で再現する手間に見合う利益が無い。
  TUI は Web UI と並ぶ第 3 のクライアントとして、PRD の「やらないこと」の「ブラウザ以外の GUI のクライアント」から除くにとどめた。
- cobra のシグナル連動の context を TUI にも渡す。
  上記のとおり、対話セッション中の Ctrl-C で context が取り消され、以後の取得が失敗する。
  セッション中だけシグナルを無視する形も検討したが、bubbletea の `tea.ExecProcess` の前後でシグナルの扱いを切り替える責任が TUI 側に生まれ、漏れが無いことを保証できない。

## 結果

- 利用者は `thief tui` だけで AWS の一覧、詳細、対話セッションを端末の中で行える。
  API サーバとブラウザは不要である。
- `bubbles` と `lipgloss` が直接依存に加わる。
  `bubbletea` は既存の直接依存であり、`atotto/clipboard` が `bubbles` の間接依存として加わる。
- TUI にサービスを足すときは、Web UI の列定義 (`frontend/src/components/tables/columns.tsx`) と Drawer の Overview (`frontend/src/components/Drawer/overviewRows.tsx`) を `backend/internal/tui/<service>.go` に書き写す必要がある。
  列と Overview の定義は backend と frontend で二重に持つ。
- TUI は API サーバのキャッシュ (ADR 0005) を使わないため、切り替えのたびに AWS の API を呼ぶ。
- 画面で押した Ctrl-C の扱いが CLI (終了コード 130) と TUI (終了コード 0) で異なる。
- 第 1 段階は AWS の一覧、Drawer (Overview と Tags)、EC2 と ECS への対話セッションに限る。
  Google Cloud、Datadog、TiDB Cloud、分析、リソースの変更操作は第 2 段階以降で扱い、段階ごとの範囲と順序は PRD の「未確定論点」として残す。
  コストと料金のビューなど材料に無いものを TUI で扱うかも、同じ論点で決める。

## 根拠資料

- PRD `docs/prd/thief.md` の FR-29、FR-30、「決定した論点」の 2026-10-23 の表
- ADR 0002、ADR 0004、ADR 0005、ADR 0012、ADR 0023、ADR 0032
- `backend/internal/tui/program.go` `Run`、`backend/internal/tui/service.go`、`backend/internal/cli/tui.go` `Deps`、`sessionManagerCommand`
- `backend/internal/util/selecter.go` `Select` (bubbletea の既存の利用)
- `backend/internal/cli/run_test.go` `TestNoRootContextOutsideDesignatedFunctions`
- bubbletea v1.3.10: https://github.com/charmbracelet/bubbletea
- bubbles v0.21.0: https://github.com/charmbracelet/bubbles
- lipgloss v1.1.0: https://github.com/charmbracelet/lipgloss
