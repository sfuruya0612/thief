// AWS リソースの Raw (JSON) / Row (UI 用) 型定義
// Raw は backend/internal/aws/*.go の JSON タグをミラーする
// Row は snake_case → camelCase 変換、cost_monthly は除外、region を必須で保持する

// ============================================================
// EC2
// ============================================================
export interface EC2Raw {
  id: string;
  name: string;
  state: string;
  instance_type: string;
  az: string;
  private_ip: string;
  public_ip: string;
  vpc_id: string;
  tags: Record<string, string>;
  cost_monthly: number;
  launch_time: string;
}

export interface EC2Row {
  region: string;
  id: string;
  name: string;
  state: string;
  instanceType: string;
  az: string;
  privateIp: string;
  publicIp: string;
  vpcId: string;
  tags: Record<string, string>;
  uptime?: string;
  launched?: string;
}

// ============================================================
// RDS
// ============================================================
export interface RDSRaw {
  id: string;
  name: string;
  state: string;
  engine: string;
  engine_version: string;
  class: string;
  multi_az: boolean;
  endpoint: string;
  port: number;
  vpc_id: string;
  tags: Record<string, string>;
  cost_monthly: number;
  launch_time: string;
}

export interface RDSRow {
  region: string;
  id: string;
  name: string;
  state: string;
  engine: string;
  engineVersion: string;
  class: string;
  multiAz: boolean;
  endpoint: string;
  port: number;
  vpcId: string;
  tags: Record<string, string>;
  uptime?: string;
  launched?: string;
}

// ============================================================
// DynamoDB
// ============================================================
export interface DynamoRaw {
  id: string;
  name: string;
  state: string;
  mode: string;
  item_count: number;
  size_bytes: number;
  gsi_count: number;
  tags: Record<string, string>;
  cost_monthly: number;
}

export interface DynamoRow {
  region: string;
  id: string;
  name: string;
  state: string;
  mode: string;
  itemCount: number;
  sizeBytes: number;
  gsiCount: number;
  tags: Record<string, string>;
}

// ============================================================
// ElastiCache (キー: cache)
// ============================================================
export interface CacheRaw {
  id: string;
  name: string;
  state: string;
  engine: string;
  engine_version: string;
  node_type: string;
  num_nodes: number;
  endpoint: string;
  port: number;
  cost_monthly: number;
}

export interface CacheRow {
  region: string;
  id: string;
  name: string;
  state: string;
  engine: string;
  engineVersion: string;
  nodeType: string;
  numNodes: number;
  endpoint: string;
  port: number;
}

// ============================================================
// Lambda
// ============================================================
export interface LambdaRaw {
  id: string;
  name: string;
  state: string;
  runtime: string;
  memory_mb: number;
  timeout_sec: number;
  handler: string;
  role: string;
  tags: Record<string, string>;
  cost_monthly: number;
}

export interface LambdaRow {
  region: string;
  id: string;
  name: string;
  state: string;
  runtime: string;
  memoryMb: number;
  timeoutSec: number;
  handler: string;
  role: string;
  tags: Record<string, string>;
}

// ============================================================
// ECS
// ============================================================
export interface ECSRaw {
  id: string;
  name: string;
  state: string;
  active_services: number;
  running_tasks: number;
  pending_tasks: number;
  registered_ec2: number;
  tags: Record<string, string>;
  cost_monthly: number;
}

export interface ECSRow {
  region: string;
  id: string;
  name: string;
  state: string;
  activeServices: number;
  runningTasks: number;
  pendingTasks: number;
  registeredEc2: number;
  tags: Record<string, string>;
}

// ============================================================
// ECS Services / Tasks / Containers (Terminal タブの Exec 対象選択に使う一覧)
// ============================================================
export interface ECSServiceRaw {
  arn: string;
  name: string;
  status: string;
  desired_count: number;
  running_count: number;
  pending_count: number;
  task_definition: string;
  launch_type: string;
}

export interface ECSServiceRow {
  arn: string;
  name: string;
  status: string;
  desiredCount: number;
  runningCount: number;
  pendingCount: number;
  taskDefinition: string;
  launchType: string;
}

export interface ECSTaskContainerDetailRaw {
  name: string;
  image: string;
  last_status: string;
  health_status: string;
  exit_code?: number;
  reason: string;
}

export interface ECSTaskContainerDetailRow {
  name: string;
  image: string;
  lastStatus: string;
  healthStatus: string;
  exitCode?: number;
  reason: string;
}

export interface ECSTaskRaw {
  arn: string;
  group: string;
  last_status: string;
  desired_status: string;
  launch_type: string;
  enable_execute_command: boolean;
  container_names: string[];
  cpu: string;
  memory: string;
  started_at: string;
  stopped_at: string;
  stopped_reason: string;
  containers: ECSTaskContainerDetailRaw[];
}

export interface ECSTaskRow {
  arn: string;
  group: string;
  lastStatus: string;
  desiredStatus: string;
  launchType: string;
  enableExecuteCommand: boolean;
  containerNames: string[];
  cpu: string;
  memory: string;
  startedAt: string;
  stoppedAt: string;
  stoppedReason: string;
  containers: ECSTaskContainerDetailRow[];
}

export interface ECSContainerRaw {
  name: string;
  runtime_id: string;
  last_status: string;
  exec_enabled: boolean;
}

export interface ECSContainerRow {
  name: string;
  runtimeId: string;
  lastStatus: string;
  execEnabled: boolean;
}

// ============================================================
// ECR
// ============================================================
export interface ECRRepoRaw {
  id: string;
  name: string;
  state: string;
  uri: string;
  created_at: string;
  image_tag_mutability: string;
  scan_on_push: boolean;
}

export interface ECRRepoRow {
  region: string;
  id: string;
  name: string;
  state: string;
  uri: string;
  createdAt: string;
  imageTagMutability: string;
  scanOnPush: boolean;
}

// ECR イメージ一覧 (Drawer の Images タブでリポジトリごとに取得するサブリソース)
export interface ECRImageRaw {
  repository_name: string;
  image_tag: string;
  image_digest: string;
  pushed_at: string;
  last_pulled_at: string;
  image_size_bytes: number;
}

export interface ECRImageRow {
  id: string;
  name: string;
  repositoryName: string;
  imageTag: string;
  imageDigest: string;
  pushedAt: string;
  lastPulledAt: string;
  imageSizeBytes: number;
}

// ============================================================
// S3
// ============================================================
export interface S3Raw {
  id: string;
  name: string;
  state: string;
  region: string;
  created_at: string;
  public: boolean;
  encryption: string;
  cost_monthly: number;
}

export interface S3Row {
  region: string;
  id: string;
  name: string;
  state: string;
  createdAt: string;
  public: boolean;
  encryption: string;
}

// ============================================================
// S3 Objects (Drawer の Objects タブ)
// ============================================================
export interface S3ObjectRaw {
  key: string;
  size: number;
  last_modified: string;
  storage_class: string;
  etag: string;
}

export interface S3ObjectRow {
  key: string;
  size: number;
  lastModified: string;
  storageClass: string;
  etag: string;
}

// ============================================================
// IAM (グローバルサービス: region は 'global' 固定)
// ============================================================
export interface IAMRaw {
  id: string;
  name: string;
  state: string;
  arn: string;
  kind: string;
  mfa_enabled: boolean;
  last_activity: string;
  groups: string[] | null;
  policies: string[] | null;
}

export interface IAMRow {
  region: string;
  id: string;
  name: string;
  state: string;
  arn: string;
  kind: string;
  mfaEnabled: boolean;
  lastActivity: string;
  groups: string[];
  policies: string[];
}

// ============================================================
// ELB
// ============================================================
export interface ELBRaw {
  id: string;
  name: string;
  state: string;
  type: string;
  scheme: string;
  dns_name: string;
  vpc_id: string;
  azs: string[] | null;
  cost_monthly: number;
}

export interface ELBRow {
  region: string;
  id: string;
  name: string;
  state: string;
  type: string;
  scheme: string;
  dnsName: string;
  vpcId: string;
  azs: string[];
}

// ============================================================
// ELB Listener / Rule / TargetGroup / TargetHealth (Drawer の Listeners / Targets タブ)
// ============================================================
export interface ELBListenerRaw {
  arn: string;
  load_balancer_arn: string;
  protocol: string;
  port: number;
  default_action_type: string;
  default_target_group_arn: string;
}

export interface ELBListenerRow {
  arn: string;
  loadBalancerArn: string;
  protocol: string;
  port: number;
  defaultActionType: string;
  defaultTargetGroupArn: string;
}

export interface ELBRuleRaw {
  arn: string;
  priority: string;
  is_default: boolean;
  conditions: string[] | null;
  action_type: string;
  target_group_arn: string;
}

export interface ELBRuleRow {
  arn: string;
  priority: string;
  isDefault: boolean;
  conditions: string[];
  actionType: string;
  targetGroupArn: string;
}

export interface ELBTargetGroupRaw {
  arn: string;
  name: string;
  protocol: string;
  port: number;
  target_type: string;
  vpc_id: string;
  health_check_path: string;
  load_balancer_arns: string[] | null;
}

export interface ELBTargetGroupRow {
  arn: string;
  name: string;
  protocol: string;
  port: number;
  targetType: string;
  vpcId: string;
  healthCheckPath: string;
  loadBalancerArns: string[];
}

export interface ELBTargetHealthRaw {
  target_id: string;
  port: number;
  availability_zone: string;
  state: string;
  reason: string;
  description: string;
}

export interface ELBTargetHealthRow {
  targetId: string;
  port: number;
  availabilityZone: string;
  state: string;
  reason: string;
  description: string;
}

// ============================================================
// CloudFront (グローバル: region は 'global' 固定)
// ============================================================
export interface CloudFrontRaw {
  id: string;
  name: string;
  state: string;
  domain_name: string;
  origins: string[] | null;
  enabled: boolean;
  price_class: string;
  cost_monthly: number;
}

export interface CloudFrontRow {
  region: string;
  id: string;
  name: string;
  state: string;
  domainName: string;
  origins: string[];
  enabled: boolean;
  priceClass: string;
}

// ============================================================
// API Gateway
// ============================================================
export interface APIGWRaw {
  id: string;
  name: string;
  state: string;
  type: string;
  stage: string;
  endpoint: string;
  tags: Record<string, string> | null;
  cost_monthly: number;
}

export interface APIGWRow {
  region: string;
  id: string;
  name: string;
  state: string;
  type: string;
  stage: string;
  endpoint: string;
  tags: Record<string, string>;
}

// ============================================================
// NAT Gateway
// ============================================================
export interface NATGWRaw {
  id: string;
  name: string;
  state: string;
  vpc_id: string;
  subnet_id: string;
  elastic_ip: string;
  tags: Record<string, string>;
  cost_monthly: number;
  launch_time: string;
}

export interface NATGWRow {
  region: string;
  id: string;
  name: string;
  state: string;
  vpcId: string;
  subnetId: string;
  elasticIp: string;
  tags: Record<string, string>;
  uptime?: string;
  launched?: string;
}

// ============================================================
// SQS
// ============================================================
export interface SQSRaw {
  id: string;
  name: string;
  state: string;
  type: string;
  available_messages: number;
  in_flight: number;
  retention_days: number;
  tags: Record<string, string>;
  cost_monthly: number;
}

export interface SQSRow {
  region: string;
  id: string;
  name: string;
  state: string;
  type: string;
  availableMessages: number;
  inFlight: number;
  retentionDays: number;
  tags: Record<string, string>;
}

// ============================================================
// Kinesis
// ============================================================
export interface KinesisRaw {
  id: string;
  name: string;
  state: string;
  shard_count: number;
  retention_hours: number;
  encryption_type: string;
  tags: Record<string, string>;
  cost_monthly: number;
}

export interface KinesisRow {
  region: string;
  id: string;
  name: string;
  state: string;
  shardCount: number;
  retentionHours: number;
  encryptionType: string;
  tags: Record<string, string>;
}

// ============================================================
// WAF
// ============================================================
export interface WAFRaw {
  id: string;
  name: string;
  state: string;
  scope: string;
  rule_count: number;
  associated_count: number;
  tags: Record<string, string>;
  cost_monthly: number;
}

export interface WAFRow {
  region: string;
  id: string;
  name: string;
  state: string;
  scope: string;
  ruleCount: number;
  associatedCount: number;
  tags: Record<string, string>;
}

// ============================================================
// SSM Parameter Store (キー: ssm)
// Value は一覧レスポンスに復号済みの値を含む (機密値の露出を許容する運用方針による)
// ============================================================
export interface SSMParamRaw {
  id: string;
  name: string;
  state: string;
  type: string;
  tier: string;
  version: number;
  last_modified: string;
  value: string;
}

export interface SSMParamRow {
  region: string;
  id: string;
  name: string;
  state: string;
  type: string;
  tier: string;
  version: number;
  lastModified: string;
  value: string;
}

// ============================================================
// Secrets Manager (キー: secrets)
// Value は一覧レスポンスに復号済みの値を含む (SSM Parameter Store と同方針)
// ============================================================
export interface SecretRaw {
  id: string;
  name: string;
  state: string;
  description: string;
  last_changed: string;
  value: string;
}

export interface SecretRow {
  region: string;
  id: string;
  name: string;
  state: string;
  description: string;
  lastChanged: string;
  value: string;
}

// ============================================================
// Region (DescribeRegions からの動的取得結果)
// ============================================================
export interface RegionRaw {
  code: string;
  name: string;
}

export interface RegionRow {
  code: string;
  name: string;
}

// ============================================================
// Cost / Forecast
// ============================================================
export interface CostRaw {
  time_period: string;
  service: string;
  unblended_amount: number;
  net_amortized_amount: number;
  unit: string;
}

export interface CostRow {
  timePeriod: string;
  service: string;
  unblendedAmount: number;
  netAmortizedAmount: number;
  unit: string;
}

export interface ForecastRaw {
  time_period: string;
  amount: number;
  unit: string;
}

export interface ForecastRow {
  timePeriod: string;
  amount: number;
  unit: string;
}
