#!/usr/bin/env bash
# floci (ローカル AWS エミュレータ) にサンプルリソースを投入する。
#
# ホストから実行する前提のため、エンドポイントはコンテナ名 (floci) ではなく
# localhost:4566 を使う。認証情報はダミー値 (floci はアカウント登録不要) を
# 環境変数で渡す。
set -euo pipefail

# Lambda のデプロイパッケージの作成に zip が要る。途中で止まると投入が中途半端になるため、
# 資源を作る前に確かめる。
if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to build the Lambda deployment package" >&2
  exit 1
fi

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

echo "==> Lambda: 関数を作成 (TUI の Lambda 一覧の確認用)"
cat > "$tmpdir/index.py" <<'EOF'
def handler(event, context):
    return {"statusCode": 200, "body": "hello from thief example"}
EOF
zip -j -q "$tmpdir/fn.zip" "$tmpdir/index.py"
aws_ lambda create-function \
  --function-name thief-example-hello \
  --runtime python3.12 \
  --handler index.handler \
  --role arn:aws:iam::000000000000:role/thief-example-lambda-role \
  --memory-size 256 \
  --timeout 30 \
  --tags Env=example \
  --zip-file "fileb://$tmpdir/fn.zip"

echo "==> ECR: リポジトリを作成 (TUI の ECR 一覧の確認用)"
aws_ ecr create-repository --repository-name thief-example/web \
  --image-tag-mutability IMMUTABLE \
  --image-scanning-configuration scanOnPush=true
aws_ ecr create-repository --repository-name thief-example/batch

echo "==> ECS: クラスタを作成 (TUI の ECS 一覧の確認用)"
aws_ ecs create-cluster --cluster-name thief-example-cluster --tags key=Env,value=example

echo "==> RDS: DB インスタンスを作成 (TUI の RDS 一覧の確認用)"
aws_ rds create-db-instance \
  --db-instance-identifier thief-example-db \
  --db-instance-class db.t3.micro \
  --engine mysql \
  --master-username admin \
  --master-user-password dummy-password \
  --allocated-storage 20 \
  --tags Key=Env,Value=example

# floci の CreateCacheCluster は memcached だけを受け付ける (redis は CreateReplicationGroup が要る)。
echo "==> ElastiCache: クラスタを作成 (TUI の ElastiCache 一覧の確認用)"
aws_ elasticache create-cache-cluster \
  --cache-cluster-id thief-example-cache \
  --engine memcached \
  --cache-node-type cache.t3.micro \
  --num-cache-nodes 1

echo "==> Kinesis: ストリームを作成 (TUI の Kinesis 一覧の確認用)"
aws_ kinesis create-stream --stream-name thief-example-stream --shard-count 2

echo "==> API Gateway: REST API と HTTP API を作成 (TUI の API Gateway 一覧の確認用)"
aws_ apigateway create-rest-api --name thief-example-api
aws_ apigatewayv2 create-api --name thief-example-http --protocol-type HTTP

# floci は既定 VPC (vpc-default-ap-northeast-1) と AZ ごとのサブネット (subnet-default-ap-northeast-1-{a,b,c}) を
# 最初から持つため、VPC は作らずそれを使う。
echo "==> NAT Gateway: Elastic IP を確保してゲートウェイを作成 (TUI の NAT Gateway 一覧の確認用)"
nat_alloc="$(aws_ ec2 allocate-address --domain vpc --query AllocationId --output text)"
aws_ ec2 create-nat-gateway \
  --subnet-id subnet-default-ap-northeast-1-a \
  --allocation-id "$nat_alloc" \
  --tag-specifications 'ResourceType=natgateway,Tags=[{Key=Name,Value=thief-example-nat}]'

echo "==> ELB: Application Load Balancer を作成 (TUI の ELB 一覧の確認用)"
aws_ elbv2 create-load-balancer \
  --name thief-example-alb \
  --subnets subnet-default-ap-northeast-1-a subnet-default-ap-northeast-1-c \
  --scheme internet-facing \
  --type application

echo "==> CloudFront: ディストリビューションを作成 (TUI の CloudFront 一覧の確認用)"
cat > "$tmpdir/distribution.json" <<'EOF'
{
  "CallerReference": "thief-example-1",
  "Comment": "thief example",
  "Enabled": true,
  "Origins": {
    "Quantity": 1,
    "Items": [
      {
        "Id": "s3-data",
        "DomainName": "thief-example-data.s3.amazonaws.com",
        "S3OriginConfig": {"OriginAccessIdentity": ""}
      }
    ]
  },
  "DefaultCacheBehavior": {
    "TargetOriginId": "s3-data",
    "ViewerProtocolPolicy": "redirect-to-https",
    "ForwardedValues": {"QueryString": false, "Cookies": {"Forward": "none"}},
    "MinTTL": 0
  }
}
EOF
aws_ cloudfront create-distribution --distribution-config "file://$tmpdir/distribution.json"

echo "==> 完了"
