# CHANGES

## develop

- [ADD] EC2 インスタンスへの SSM Start Session をブラウザから開始できるようにする
  - @sfuruya0612
- [ADD] ECS タスクコンテナへの Exec Command をブラウザから開始できるようにする
  - @sfuruya0612
- [ADD] ECR のリポジトリ一覧とイメージタグ一覧を閲覧できるようにする
  - @sfuruya0612
- [ADD] SSM Parameter Store のパラメータ Key / Value を閲覧できるようにする
  - @sfuruya0612
- [ADD] Secrets Manager のシークレット Key / Value を閲覧できるようにする
  - @sfuruya0612
- [ADD] サイドバー幅をドラッグで変更できるようにする
  - @sfuruya0612
- [CHANGE] Drawer のタブ構成を Overview / Tags のみに統一し、EC2 と ECS にのみ Terminal タブを追加する
  - @sfuruya0612
- [CHANGE] フッターを全ビュー共通で画面最下部に固定し、ウィジェットをブラウザ幅に追従させる
  - @sfuruya0612
- [CHANGE] AWS Profile / Region のセレクターをトップバーからサイドバーへ移設し、profile 表示の重複と Region pill の重複表示を解消する
  - @sfuruya0612
- [CHANGE] 機能していないトップバーのサーチバーを削除する
  - @sfuruya0612
- [FIX] SSM/ECS データチャネルの AgentMessage デシリアライズで payload digest mismatch が発生する不具合を修正する
  - @sfuruya0612
- [FIX] SSM/ECS データチャネルで送信する AgentMessage の CreatedDate が未設定のため agent 側の Validate に拒否され、シーケンス番号が不整合になる不具合を修正する
  - @sfuruya0612
- [FIX] Terminal タブでセッションを開き直すと xterm.js にフォーカスが移らず入力できなくなる不具合を修正する
  - @sfuruya0612
