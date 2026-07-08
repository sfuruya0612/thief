// app.jsx StatusBar の移植
// 全ビュー共通フッターとして App.tsx ルートに配置するため、サービスごとの模擬 CLI コマンド文言のみを表示する
// (件数は ServicePanel 内部の filtered state に依存し App まで持ち上げるとコストが大きいため、
//  課題 4-2 の方針「最小変更」に従い件数表示は廃止する)
export interface StatusBarProps {
  service: string;
}

const COMMANDS: Record<string, string> = {
  ec2: 'aws ec2 describe-instances',
  ecr: 'aws ecr describe-repositories',
  rds: 'aws rds describe-db-instances',
  cache: 'aws elasticache describe-cache-clusters',
  lambda: 'aws lambda list-functions',
  ecs: 'aws ecs list-tasks',
  s3: 'aws s3api list-buckets',
  elb: 'aws elbv2 describe-load-balancers',
  cloudfront: 'aws cloudfront list-distributions',
  apigw: 'aws apigatewayv2 get-apis',
  natgw: 'aws ec2 describe-nat-gateways',
  sqs: 'aws sqs list-queues',
  kinesis: 'aws kinesis list-streams',
  waf: 'aws wafv2 list-web-acls',
  dynamo: 'aws dynamodb list-tables',
  iam: 'aws iam list-users',
  ssm: 'aws ssm describe-parameters',
  secrets: 'aws secretsmanager list-secrets',
  bigquery: 'bq ls',
  datadog: 'datadog-ci cost estimated',
  tidb: 'ticloud cluster list',
};

export function StatusBar({ service }: StatusBarProps) {
  return (
    <div className="statusbar">
      <span className="cmd">$</span>
      <span>
        <span className="muted">{COMMANDS[service] ?? 'aws iam list-users'}</span>
      </span>
      <span className="spacer" />
    </div>
  );
}
