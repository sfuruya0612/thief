#!/usr/bin/env bash
# floci (ローカル AWS エミュレータ) にサンプルリソースを投入する。
#
# ホストから実行する前提のため、エンドポイントはコンテナ名 (floci) ではなく
# localhost:4566 を使う。認証情報はダミー値 (floci はアカウント登録不要) を
# 環境変数で渡す。
set -euo pipefail

ENDPOINT_URL="http://localhost:4566"
export AWS_ACCESS_KEY_ID="test"
export AWS_SECRET_ACCESS_KEY="test"
export AWS_DEFAULT_REGION="ap-northeast-1"

aws_() {
  aws --endpoint-url "$ENDPOINT_URL" "$@"
}

echo "==> S3: バケットとサンプルオブジェクトを作成"
aws_ s3api create-bucket --bucket thief-example-data \
  --create-bucket-configuration LocationConstraint=ap-northeast-1
aws_ s3api create-bucket --bucket thief-example-logs \
  --create-bucket-configuration LocationConstraint=ap-northeast-1

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

cat > "$tmpdir/sample.csv" <<'EOF'
id,name,amount
1,alice,1000
2,bob,2000
3,carol,3000
EOF

cat > "$tmpdir/sample.txt" <<'EOF'
thief example environment (floci)
seeded by example/seed.sh
EOF

cat > "$tmpdir/sample.json" <<'EOF'
{
  "service": "thief",
  "environment": "example",
  "emulator": "floci"
}
EOF

aws_ s3 cp "$tmpdir/sample.csv" s3://thief-example-data/reports/sample.csv
aws_ s3 cp "$tmpdir/sample.txt" s3://thief-example-data/notes/sample.txt
aws_ s3 cp "$tmpdir/sample.json" s3://thief-example-data/config/sample.json
aws_ s3 cp "$tmpdir/sample.txt" s3://thief-example-logs/app/sample.txt

echo "==> DynamoDB: テーブルとアイテムを作成"
aws_ dynamodb create-table \
  --table-name thief-example-users \
  --attribute-definitions AttributeName=id,AttributeType=S \
  --key-schema AttributeName=id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST

aws_ dynamodb put-item --table-name thief-example-users \
  --item '{"id": {"S": "1"}, "name": {"S": "alice"}, "age": {"N": "30"}}'
aws_ dynamodb put-item --table-name thief-example-users \
  --item '{"id": {"S": "2"}, "name": {"S": "bob"}, "age": {"N": "25"}}'
aws_ dynamodb put-item --table-name thief-example-users \
  --item '{"id": {"S": "3"}, "name": {"S": "carol"}, "age": {"N": "40"}}'

echo "==> SQS: キューを作成"
aws_ sqs create-queue --queue-name thief-example-queue
aws_ sqs create-queue --queue-name thief-example-dlq
aws_ sqs create-queue --queue-name thief-example-fifo.fifo --attributes FifoQueue=true

echo "==> SSM Parameter Store: パラメータを作成"
aws_ ssm put-parameter --name "/thief/example/app-name" --value "thief" --type String --overwrite
aws_ ssm put-parameter --name "/thief/example/max-connections" --value "100" --type String --overwrite
aws_ ssm put-parameter --name "/thief/example/db-password" --value "dummy-password" --type SecureString --overwrite

echo "==> Secrets Manager: シークレットを作成"
aws_ secretsmanager create-secret \
  --name thief/example/db-credentials \
  --secret-string '{"username":"admin","password":"dummy-password"}' \
  || aws_ secretsmanager put-secret-value \
    --secret-id thief/example/db-credentials \
    --secret-string '{"username":"admin","password":"dummy-password"}'

echo "==> CloudFormation: 最小スタックを作成"
cat > "$tmpdir/stack.yaml" <<'EOF'
AWSTemplateFormatVersion: '2010-09-09'
Description: thief example stack (floci)
Resources:
  ExampleBucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: thief-example-cfn-bucket
EOF

aws_ cloudformation create-stack \
  --stack-name thief-example-stack \
  --template-body "file://$tmpdir/stack.yaml"

echo "==> 完了"
