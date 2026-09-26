# ecs execute-command が session-manager-plugin 終了後に SSM セッションを切断せず AWS 側に残す

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

issue 0131 (CLI コマンドがシグナル連動 context を持たない) の作業中に、`backend/internal/cli/ecs.go` の `ecsExecuteCommand` が SSM セッションの後始末を行っていないことが判明した。

同じ session-manager-plugin を起動する経路のうち、後始末を行っているのは以下である。

- `backend/internal/cli/ec2.go` の `startEC2SessionWith`: `util.ExecCommand` の後に専用の短命 context で `TerminateSSMSession` を呼ぶ
- `backend/internal/session/bridge.go` の `cleanup`: 同様に専用の短命 context で `TerminateSSMSession` を呼ぶ
- `backend/internal/api/handlers_session.go`: セッション確立に失敗した場合の後始末で `TerminateSSMSession` を呼ぶ

`ecsExecuteCommand` にはこれが無い。

```go
	if err = util.ExecCommand(plug, string(sessJSON), cfg.Region, "StartSession", cfg.Profile, string(targetJSON), fmt.Sprintf("https://ecs.%s.amazonaws.com", cfg.Region)); err != nil {
		return fmt.Errorf("execute session-manager-plugin command: %w", err)
	}

	return nil
}
```

## 問題

`ExecuteECSCommandSession` は AWS 側にセッションを作る。session-manager-plugin が終了しても、`TerminateSSMSession` を呼ばない限りそのセッションは AWS 側の管理下に残る。

`util.ExecCommand` は実行中の SIGINT を握りつぶして子プロセスへ委ねる。利用者が Ctrl-C や `exit` でシェルを抜けた場合、plugin は正常終了して `ExecCommand` は nil を返し、`ecsExecuteCommand` はそのまま return する。セッションは切断されないまま残る。

`ExecCommand` が失敗した場合も同じで、エラーを返す前に切断を試みていない。

## 再現手順

1. ECS Exec が有効なクラスターとタスクを用意する。
2. `thief ecs execute-command --cluster <cluster> --task <task> --container <container>` を実行する。
3. 起動したシェルで `exit` を実行する (または Ctrl-C で抜ける)。
4. `aws ssm describe-sessions --state Active` を実行する。

手順 4 のセッション一覧に、手順 2 で作ったセッションが残る。`thief ec2 session` で同じ手順を踏んだ場合は残らない。

## なぜ対応が必要か

- 残ったセッションは AWS 側のアイドルタイムアウト (既定 20 分) まで Active のまま数えられる。同時セッション数の上限に近い運用では、実際には使われていないセッションが枠を占める。
- CloudTrail と Session Manager の履歴に、終了時刻が実際の利用終了と一致しないセッションが残り、監査で追いにくくなる。
- 同じ性質の経路 3 箇所が後始末を行っている中で、ここだけが行っていない。整合していない。

## 修正方針

`startEC2SessionWith` と同じ形で、`util.ExecCommand` の後に専用の短命 context で `TerminateSSMSession` を呼ぶ。

`ec2.go` にあるコメントの通り、コマンドの context (`commandContext(cmd)`) は Ctrl-C でキャンセル済みになっているため使えない。`context.WithTimeout(context.Background(), ...)` で作る必要がある。この経路は `internal/cli` の本番コードが根の context を作る場所を固定する `TestNoRootContextOutsideDesignatedFunctions` の一覧に追記する対象になる。

切断の失敗は `ExecCommand` の結果を上書きしないこと。`startEC2SessionWith` が採っている扱いに揃える。

`ecsExecuteCommand` は現在テストから差し替えられる形になっていない (`lookupSessionManagerPlugin` と `util.ExecCommand` を直接呼ぶ)。`startEC2SessionWith` が持つ `deps` と同じ差し込み口を作り、切断が呼ばれることをテストで検証する。

## 完了条件

- `ecsExecuteCommand` が session-manager-plugin の終了後に `TerminateSSMSession` を呼ぶ。
- `ExecCommand` が失敗した場合も切断を試みる。
- 切断に使う context がコマンドの context のキャンセルに影響されない。
- 切断の失敗が `ExecCommand` の結果を上書きしない。
- 上記をテストで検証する (差し込み口を作る)。
- `TestNoRootContextOutsideDesignatedFunctions` の一覧を更新する。
- `mise run check` が通る。

## 補足: 切断方法の確認 (2026-08-09)

ECS Exec のセッションは `ec2 session` と API 系統が違う (`ecs:ExecuteCommand` で作られ、`ssm:StartSession` は通らない)。`TerminateSSMSession` (`ssm:TerminateSession`) でこのセッションを切れるかを確認した。

`backend/internal/api/handlers_session.go` の `handleECSExec` が、既に同じ方法で切断している。

```go
	result, err := awsinternal.ExecuteECSCommand(r.Context(), profile, region, cluster, task, container, command)
	...
	s.runSessionBridge(w, r, result, func(ctx context.Context) error {
		return awsinternal.TerminateSSMSession(ctx, profile, region, result.SessionID)
	})
```

`ExecuteECSCommand` (Web 用) と `ExecuteECSCommandSession` (CLI 用、`backend/internal/aws/ecs_cli.go:305`) はどちらも `ecs:ExecuteCommand` の戻り値の `Session` を使う。セッション ID の出所が同じであるため、Web 側で通っている切断は CLI 側でも通る。

したがって修正方針に挙げた `TerminateSSMSession` の呼び出しはそのまま使える。必要な IAM 権限 (`ssm:TerminateSession`) も Web 側が既に要求しているものと同じであり、CLI に新たな権限要件は増えない。

## 関連

- docs/issues/0131: 起票元。CLI コマンドへのシグナル連動 context の導入。
- `backend/internal/cli/ec2.go` の `startEC2SessionWith`: 同じ後始末の実装。
- `backend/internal/api/handlers_session.go` の `handleECSExec`: ECS Exec のセッションを `TerminateSSMSession` で切っている先例。
- docs/issues/0136: 同じ 5 秒の切断タイムアウトが 3 箇所に重複している問題。本 issue で 4 箇所目を作らないよう、あわせて検討する。
- docs/issues/0142: 本 issue の多観点レビューで発見した、`ec2.go` の `startEC2SessionWith` に存在する同型の欠陥 (後述)。

## 解決方法

`backend/internal/cli/ecs.go` の `ecsExecuteCommand` を薄いラッパーにし、本体を `ecsExecuteCommandWith(cmd *cobra.Command, deps ecsExecSessionDeps) error` に切り出した。`ecsExecSessionDeps` は `executeSession` / `lookupPlugin` / `execPlugin` / `terminateSession` の 4 つの関数値を持つ構造体で、`defaultECSExecSessionDeps()` が本番の実装 (`awsinternal.ExecuteECSCommandSession` / `lookupSessionManagerPlugin` / `util.ExecCommand` / `awsinternal.TerminateSSMSession`) を配線する。`ec2.go` の `ec2SessionDeps` / `startEC2SessionWith` と同じ差し込み口の作り方である。

完了条件との対応は次のとおり。

- **`ecsExecuteCommand` が session-manager-plugin の終了後に `TerminateSSMSession` を呼ぶ**: `execPlugin` 実行後に必ず `deps.terminateSession` を呼ぶようにした。`TestECSExecuteCommandSendsPluginArgsAndTerminates` で検証する。
- **`ExecCommand` が失敗した場合も切断を試みる**: `execPlugin` の戻り値に関わらず切断を試みる。`TestECSExecuteCommandKeepsExecErrorChain` で、切断が成功する経路と失敗する経路の両方について、`*exec.ExitError` へ `errors.As` で到達できることを検証する。
- **切断に使う context がコマンドの context のキャンセルに影響されない**: `context.WithTimeout(context.Background(), ec2TerminateTimeout)` で独立させた。`TestECSExecuteCommandTerminatesWithLiveContextAfterInterrupt` で、コマンドの context を事前にキャンセルした状態でも切断用 context が生存していることを検証する。
- **切断の失敗が `ExecCommand` の結果を上書きしない**: `execErr` と `termErr` を両方 `%w` で包み、両方に `errors.Is` / `errors.As` で到達できるようにした。
- **上記をテストで検証する (差し込み口を作る)**: `ecsExecSessionDeps` が差し込み口であり、`ecs_test.go` に 10 個のテスト関数を追加した (`TestDefaultECSExecSessionDepsIsFullyWired` で本番配線の nil 漏れも検出する)。
- **`TestNoRootContextOutsideDesignatedFunctions` の一覧を更新する**: `run_test.go` の `want` スライスに `"ecs.go:ecsExecuteCommandWith"` を、ファイル名の辞書順 (`ec2.go` → `ecs.go` → `run.go` → `server.go`) を保つ位置に追加した。
- **`mise run check` が通る**: 通過を確認した。

### 修正方針からの乖離 (方式は保ったまま)

方針セクションは「`startEC2SessionWith` と同じ形で、`util.ExecCommand` の後に専用の短命 context で `TerminateSSMSession` を呼ぶ」としていたが、実装は当初これをそのまま踏襲し、`termCtx` の生成を `execPlugin` の呼び出し直後に置いていた。

多観点レビューの堅牢性観点で、この形だと `deps.executeSession` 成功後、`sessJSON` / `targetJSON` の marshal 失敗や `deps.lookupPlugin()` の失敗で早期 return した場合に `deps.terminateSession` が一度も呼ばれず、確立済みのセッションが残ってしまう高優先度の指摘を受けた。特に `lookupPlugin` の失敗 (session-manager-plugin が PATH に無い) は現実的に起こり得る。

これを反映し、`termCtx` / `cancelTerm` の生成を `deps.executeSession` 成功直後まで前倒しし、以降のどの失敗経路 (marshal session、marshal target、plugin 探索、plugin 実行) でも `terminateThen` というクロージャで切断を試みてから return するように変更した。これは方針が明示していなかった範囲への拡張であり、`ec2.go` の `startEC2SessionWith` の現行実装には無い挙動である (`startEC2SessionWith` は `execPlugin` 呼び出し後にしか切断を試みず、同じ欠陥を持つ)。この欠陥は本 issue のスコープ外 (対象は `ecs.go`) のため、docs/issues/0142 として別途登録し、`ec2.go` 側の修正はそちらに委ねた。

反映後、`TestECSExecuteCommandPropagatesSetupErrors` の `plugin lookup fails` ケースを、切断が呼ばれないことの検証から呼ばれることの検証に更新し、切断も失敗した場合の挙動を検証する `TestECSExecuteCommandTerminatesOnLookupPluginFailureAfterSessionEstablished` を追加した。

### 多観点レビューの結果

5 観点 × 1 ラウンド (堅牢性観点の指摘反映により、完了条件充足とテスト品質の 2 観点のみ変更点に限定した追加レビューを 1 回実施) を行った。

- 観点1 (完了条件充足): 指摘なし。
- 観点2 (テスト品質): 指摘なし。ミューテーションテスト 5 種 (エラーラップの劣化、切断用 context の取り違え、切断呼び出しの欠落、必須フラグ判定の論理演算子誤り、依存関数への引数順の取り違え) を実際に埋め込み、対応するテストがすべて検出することを確認した。
- 観点3 (堅牢性): 高優先度 1 件 (上記「修正方針からの乖離」で反映)。中優先度 1 件 (`ec2TerminateTimeout` の共有が ecs.go 側のコメントにしか無く片方向) は、docs/issues/0136 (切断タイムアウトの重複) で追跡済みの設計判断のため、コメントの追記のみでは根本対応にならないと判断し、docs/issues/0136 側での対応に委ねることとして却下した (根本対応は定数の集約であり、コメントの双方向化は表面的な緩和にとどまるため)。
- 観点4 (規約準拠): 指摘なし。
- 観点5 (回帰と整合): 中優先度 2 件。CHANGES.md の記述が `ecsExecuteCommand` と `ecsExecuteCommandWith` を混同していた点を修正し、実装が `ec2.go` の現行実装をそのまま模倣したのではなく上回る修正をした経緯が issue 本文に反映されていない点は、本セクション (「修正方針からの乖離」) の追記で反映した。

反映後、追加レビュー (完了条件充足・テスト品質) を実施し指摘なしを確認した。反映しきれない高優先度の指摘は残っていない。

### 補足

`ecsExecuteCommand` / `startEC2SessionWith` を含め、他のコードからの参照は `backend/internal/cli/ecs.go` と `backend/internal/cli/ecs_test.go` に閉じており、回帰は確認されなかった (`go build ./...`、`go test -race ./...` (全パッケージ) とも成功)。
