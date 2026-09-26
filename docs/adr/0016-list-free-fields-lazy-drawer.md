# 0016. 一覧には主な Describe が返す項目だけを載せ、追加の取得は Drawer で行う

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

AWS のリソースには、一覧の API (主な Describe) が返す項目と、別の API を呼ばないと取れない項目がある。
後者を一覧に載せると、項目ごとに API の呼び出しが増え、追加の IAM 権限も要り、一覧が遅くなる。

## 決定

- 一覧には、原則として、主な Describe が追加の課金と追加の権限なしで返す項目だけを載せる。
- 例外として、次のサービスは一覧の列のために項目ごとに追加の API を呼ぶ。
  - WAF (Rules、Associated) と IAM (MFA、グループ、ポリシー)。
    同時実行数は 10 に制限する (`backend/internal/aws/waf.go` `wafACLConcurrency`、`backend/internal/aws/iam.go` `iamDetailConcurrency`、ADR 0015)。
  - S3 (Region、Public、Encryption)。
    バケットごとに `GetBucketLocation`、`GetBucketEncryption`、`GetPublicAccessBlock` を呼ぶ (`backend/internal/aws/s3.go`)。
- DynamoDB、SQS、WAF は、一覧の取得のときに項目ごとにタグも取得する。
  タグは一覧の列には出さず、Drawer の Tags タブで表示する。
- パラメータ、イメージ、ターゲット、イベントなどの従属するリソースは、Drawer のタブを開いたときに取得する。
- Drawer のタブの取得に失敗したら、空の表示にせず、エラーを表示する (`DrawerError`、issue 0075)。

## 検討した代替案

方針全体の採否を比べた記録は無い。
従属するリソースを Drawer で取得する形は、最初の実装 (コミット 2aac22f、2026-07-08) の ECR の Images タブからある。
issue 0070 は、この方針を既存のサービス (ECR Images、ELB Targets、CFN Resources、issue 0066 の Parameters タブ) で統一済みの方針として扱っている。
追加の取得が要る情報 (RDS のクラスタの Writer と Reader の役割など) は、この方針とは切り分けて個別に判断するとした。
個別の却下案として、issue 0070 は `DescribeDBClusters` でクラスタ自体を一覧の行に加える案を、新しい API の呼び出しと権限への依存を持ち込むため採らなかった。
issue 0075 は、WAF のルールの一覧を一覧の応答 (`WAFResource`) に埋め込む案を、応答が大きくなるため採らなかった。

## 結果

- WAF、IAM、S3 の列と、DynamoDB、SQS、WAF のタグを除き、一覧の表示はサービスごとに 1 系統の API 呼び出しで済む。
- 一覧で絞り込めない項目がある。
  その項目を見るには Drawer を開く。
- 項目の一部の取得に失敗したときは、`*_fetch_failed` のフラグで行ごとに示す (issue 0091、0109)。

## 根拠資料

- コミット 2aac22f (2026-07-08)
- `docs/issues/closed/0066`、`0070`、`0071`、`0072`、`0075`、`0088`、`0091`、`0109`
- `backend/internal/aws/s3.go`
- `frontend/src/components/Drawer/Drawer.tsx` `DRAWER_TABS`、`frontend/src/components/Drawer/drawerError.tsx`
