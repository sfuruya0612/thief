import type { ServiceGroupMeta, ServiceMeta } from '../types/common';

// AWS の公式プロダクトカテゴリ。配列順がサイドバーのセクション表示順になる。
export const AWS_SERVICE_GROUPS: ServiceGroupMeta[] = [
  { key: 'compute', label: 'Compute' },
  { key: 'containers', label: 'Containers' },
  { key: 'storage', label: 'Storage' },
  { key: 'database', label: 'Database' },
  { key: 'networking', label: 'Networking & Content Delivery' },
  { key: 'analytics', label: 'Analytics' },
  { key: 'integration', label: 'Application Integration' },
  { key: 'security', label: 'Security, Identity, & Compliance' },
  { key: 'management', label: 'Management & Governance' },
  { key: 'cost', label: 'Cloud Financial Management' },
];

// Google Cloud の公式プロダクトカテゴリ。配列順がサイドバーのセクション表示順になる。
export const GCP_SERVICE_GROUPS: ServiceGroupMeta[] = [
  { key: 'compute', label: 'Compute' },
  { key: 'analytics', label: 'Data Analytics' },
  { key: 'storage', label: 'Storage' },
  { key: 'security', label: 'Security & Identity' },
  { key: 'observability', label: 'Observability' },
];

// data.jsx SERVICES を移植。色は app.css の --svc-* CSS 変数を参照する
export const SERVICES: ServiceMeta[] = [
  { key: 'ec2', name: 'EC2', sub: 'Instances', color: 'var(--svc-ec2)', group: 'compute' },
  { key: 'lambda', name: 'Lambda', sub: 'Functions', color: 'var(--svc-lambda)', group: 'compute' },
  { key: 'ecr', name: 'ECR', sub: 'Repositories', color: 'var(--svc-ecr)', group: 'containers' },
  { key: 'ecs', name: 'ECS', sub: 'Tasks', color: 'var(--svc-ecs)', group: 'containers' },
  { key: 's3', name: 'S3', sub: 'Buckets', color: 'var(--svc-s3)', group: 'storage' },
  { key: 'rds', name: 'RDS', sub: 'Databases', color: 'var(--svc-rds)', group: 'database' },
  { key: 'dynamo', name: 'DynamoDB', sub: 'Tables', color: 'var(--svc-dynamo)', group: 'database' },
  {
    key: 'cache',
    name: 'ElastiCache',
    sub: 'Clusters',
    color: 'var(--svc-cache)',
    group: 'database',
  },
  { key: 'elb', name: 'ELB', sub: 'Load balancers', color: 'var(--svc-elb)', group: 'networking' },
  {
    key: 'cloudfront',
    name: 'CloudFront',
    sub: 'Distributions',
    color: 'var(--svc-cf)',
    group: 'networking',
  },
  {
    key: 'apigw',
    name: 'API Gateway',
    sub: 'APIs',
    color: 'var(--svc-apigw)',
    group: 'networking',
  },
  {
    key: 'natgw',
    name: 'NAT Gateway',
    sub: 'Gateways',
    color: 'var(--svc-natgw)',
    group: 'networking',
  },
  {
    key: 'athena',
    name: 'Athena',
    sub: 'Query editor',
    color: 'var(--svc-athena)',
    group: 'analytics',
  },
  {
    key: 'kinesis',
    name: 'Kinesis',
    sub: 'Streams',
    color: 'var(--svc-kinesis)',
    group: 'analytics',
  },
  { key: 'sqs', name: 'SQS', sub: 'Queues', color: 'var(--svc-sqs)', group: 'integration' },
  { key: 'iam', name: 'IAM', sub: 'Users&Roles', color: 'var(--svc-iam)', group: 'security' },
  { key: 'waf', name: 'WAF', sub: 'Web ACLs', color: 'var(--svc-waf)', group: 'security' },
  {
    key: 'secrets',
    name: 'Secrets Manager',
    sub: 'Secrets',
    color: 'var(--svc-secrets)',
    group: 'security',
  },
  {
    key: 'ssm',
    name: 'Parameter Store',
    sub: 'Parameters',
    color: 'var(--svc-ssm)',
    group: 'management',
  },
  {
    key: 'cfn',
    name: 'CloudFormation',
    sub: 'Stacks',
    color: 'var(--svc-cfn)',
    group: 'management',
  },
  {
    key: 'costexplorer',
    name: 'Cost Explorer',
    sub: 'Cost & Usage',
    color: 'var(--svc-costexplorer)',
    group: 'cost',
  },
];

// GCP サービス一覧
export const GCP_SERVICES: ServiceMeta[] = [
  {
    key: 'cloudrun',
    name: 'Cloud Run',
    sub: 'Services & Jobs',
    color: 'var(--svc-cloudrun)',
    group: 'compute',
  },
  {
    key: 'bigquery',
    name: 'BigQuery',
    sub: 'Query editor',
    color: 'var(--svc-bigquery)',
    group: 'analytics',
  },
  { key: 'gcs', name: 'Cloud Storage', sub: 'Buckets', color: 'var(--svc-gcs)', group: 'storage' },
  {
    key: 'gcpiam',
    name: 'IAM',
    sub: 'Bindings',
    color: 'var(--svc-gcpiam)',
    group: 'security',
  },
  {
    key: 'gcpserviceaccounts',
    name: 'Service Accounts',
    sub: 'Accounts',
    color: 'var(--svc-gcpserviceaccounts)',
    group: 'security',
  },
  {
    key: 'cloudlogging',
    name: 'Cloud Logging',
    sub: 'Log entries',
    color: 'var(--svc-cloudlogging)',
    group: 'observability',
  },
];

// GCP サービスキー → バックエンド URL パスセグメント
// bigquery は埋め込みビュー (既存 useBQDatasets 等を再利用) のためパスなし。
// cloudlogging も埋め込みビューだが、専用の getGcpLogEntries/useGcpLogEntries が直接
// '/api/gcp/logging/entries' を叩くため、この対応関係は他所から参照されない (将来の
// 一貫性のためだけに記載する)。
export const GCP_SERVICE_TO_PATH: Record<string, string> = {
  cloudrun: 'cloudrun',
  gcs: 'gcs',
  gcpiam: 'iam',
  gcpserviceaccounts: 'serviceaccounts',
  cloudlogging: 'logging',
};

// サービスキー → バックエンド URL パスセグメント
export const SERVICE_TO_PATH: Record<string, string> = {
  ec2: 'ec2',
  ecr: 'ecr',
  rds: 'rds',
  dynamo: 'dynamo',
  cache: 'elasticache',
  lambda: 'lambda',
  ecs: 'ecs',
  s3: 's3',
  iam: 'iam',
  elb: 'elb',
  cloudfront: 'cloudfront',
  apigw: 'apigw',
  natgw: 'natgw',
  sqs: 'sqs',
  kinesis: 'kinesis',
  waf: 'waf',
  ssm: 'ssm/parameters',
  secrets: 'secretsmanager',
  cfn: 'cfn/stacks',
};
