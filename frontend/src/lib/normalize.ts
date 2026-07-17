// Raw (バックエンド JSON) → Row (UI 用) 変換関数
import { formatUptime } from './format';
import type {
  CallerIdentity,
  CallerIdentityRaw,
  Profile,
  ProfileAuthType,
  ProfileRaw,
  ProfileSSOStatus,
} from '../types/common';
import type {
  APIGWRaw,
  APIGWRow,
  CacheRaw,
  CacheRow,
  CFNStackDetailRaw,
  CFNStackDetailRow,
  CFNStackEventRaw,
  CFNStackEventRow,
  CFNStackRaw,
  CFNStackResourceRaw,
  CFNStackResourceRow,
  CFNStackRow,
  CloudFrontRaw,
  CloudFrontRow,
  DynamoIndexSchemaRaw,
  DynamoIndexSchemaRow,
  DynamoRaw,
  DynamoRow,
  DynamoTableSchemaRaw,
  DynamoTableSchemaRow,
  EC2Raw,
  EC2Row,
  ECRImageRaw,
  ECRImageRow,
  ECRRepoRaw,
  ECRRepoRow,
  ECSContainerRaw,
  ECSContainerRow,
  ECSRaw,
  ECSRow,
  ECSServiceRaw,
  ECSServiceRow,
  ECSTaskRaw,
  ECSTaskRow,
  ELBListenerRaw,
  ELBListenerRow,
  ELBRaw,
  ELBRow,
  ELBRuleRaw,
  ELBRuleRow,
  ELBTargetGroupRaw,
  ELBTargetGroupRow,
  ELBTargetHealthRaw,
  ELBTargetHealthRow,
  IAMRaw,
  IAMRow,
  KinesisRaw,
  KinesisRow,
  LambdaRaw,
  LambdaRow,
  NATGWRaw,
  NATGWRow,
  RDSRaw,
  RDSRow,
  S3ObjectRaw,
  S3ObjectRow,
  S3Raw,
  S3Row,
  SecretRaw,
  SecretRow,
  SQSRaw,
  SQSRow,
  SSMParamRaw,
  SSMParamRow,
  WAFRaw,
  WAFRow,
} from '../types/aws';

// launch_time が有効な ISO 日時であれば YYYY-MM-DD を返す
function launchedDate(iso: string): string | undefined {
  if (!iso) return undefined;
  const d = new Date(iso);
  if (isNaN(d.getTime())) return undefined;
  return d.toISOString().slice(0, 10);
}

function uptimeOrUndef(iso: string): string | undefined {
  if (!iso) return undefined;
  const u = formatUptime(iso);
  return u || undefined;
}

const PROFILE_AUTH_TYPES: readonly ProfileAuthType[] = [
  'sso',
  'access_key',
  'assume_role',
  'credential_process',
  'unknown',
];
const PROFILE_SSO_STATUSES: readonly ProfileSSOStatus[] = ['valid', 'expired', 'not_logged_in'];

// backend の enum 文字列を union 型へ縮小する。未知の値 (将来の backend 拡張等)
// は undefined に落とし、UI 側はバッジ等を出さない挙動に degrade する。
function narrowEnum<T extends string>(
  value: string | undefined,
  allowed: readonly T[],
): T | undefined {
  return allowed.includes(value as T) ? (value as T) : undefined;
}

export function profileFromRaw(raw: ProfileRaw): Profile {
  return {
    name: raw.name,
    accountId: raw.account_id,
    ssoRoleName: raw.sso_role_name,
    region: raw.region,
    authType: narrowEnum(raw.auth_type, PROFILE_AUTH_TYPES),
    ssoStatus: narrowEnum(raw.sso_status, PROFILE_SSO_STATUSES),
    ssoExpiresAt: raw.sso_expires_at,
  };
}

export function callerIdentityFromRaw(raw: CallerIdentityRaw): CallerIdentity {
  return {
    accountId: raw.account_id,
    arn: raw.arn,
    userId: raw.user_id,
  };
}

export function ec2FromRaw(raw: EC2Raw, region: string): EC2Row {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    instanceType: raw.instance_type,
    az: raw.az,
    privateIp: raw.private_ip,
    publicIp: raw.public_ip,
    vpcId: raw.vpc_id,
    tags: raw.tags ?? {},
    uptime: uptimeOrUndef(raw.launch_time),
    launched: launchedDate(raw.launch_time),
  };
}

export function rdsFromRaw(raw: RDSRaw, region: string): RDSRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    engine: raw.engine,
    engineVersion: raw.engine_version,
    class: raw.class,
    multiAz: raw.multi_az,
    endpoint: raw.endpoint,
    port: raw.port,
    vpcId: raw.vpc_id,
    tags: raw.tags ?? {},
    uptime: uptimeOrUndef(raw.launch_time),
    launched: launchedDate(raw.launch_time),
  };
}

export function dynamoFromRaw(raw: DynamoRaw, region: string): DynamoRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    mode: raw.mode,
    itemCount: raw.item_count,
    sizeBytes: raw.size_bytes,
    gsiCount: raw.gsi_count,
    tags: raw.tags ?? {},
  };
}

export function dynamoIndexSchemaFromRaw(raw: DynamoIndexSchemaRaw): DynamoIndexSchemaRow {
  return {
    name: raw.name,
    partitionKey: { name: raw.partition_key.name, type: raw.partition_key.type },
    sortKey: raw.sort_key ? { name: raw.sort_key.name, type: raw.sort_key.type } : undefined,
  };
}

export function dynamoTableSchemaFromRaw(raw: DynamoTableSchemaRaw): DynamoTableSchemaRow {
  return {
    tableName: raw.table_name,
    table: dynamoIndexSchemaFromRaw(raw.table),
    gsis: (raw.gsis ?? []).map(dynamoIndexSchemaFromRaw),
  };
}

export function cacheFromRaw(raw: CacheRaw, region: string): CacheRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    engine: raw.engine,
    engineVersion: raw.engine_version,
    nodeType: raw.node_type,
    numNodes: raw.num_nodes,
    endpoint: raw.endpoint,
    port: raw.port,
  };
}

export function lambdaFromRaw(raw: LambdaRaw, region: string): LambdaRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    runtime: raw.runtime,
    memoryMb: raw.memory_mb,
    timeoutSec: raw.timeout_sec,
    handler: raw.handler,
    role: raw.role,
    tags: raw.tags ?? {},
  };
}

export function ecsFromRaw(raw: ECSRaw, region: string): ECSRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    activeServices: raw.active_services,
    runningTasks: raw.running_tasks,
    pendingTasks: raw.pending_tasks,
    registeredEc2: raw.registered_ec2,
    tags: raw.tags ?? {},
  };
}

export function ecsServiceFromRaw(raw: ECSServiceRaw): ECSServiceRow {
  return {
    arn: raw.arn,
    name: raw.name,
    status: raw.status,
    desiredCount: raw.desired_count,
    runningCount: raw.running_count,
    pendingCount: raw.pending_count,
    taskDefinition: raw.task_definition,
    launchType: raw.launch_type,
  };
}

export function ecsTaskFromRaw(raw: ECSTaskRaw): ECSTaskRow {
  return {
    arn: raw.arn,
    group: raw.group,
    lastStatus: raw.last_status,
    desiredStatus: raw.desired_status,
    launchType: raw.launch_type,
    enableExecuteCommand: raw.enable_execute_command,
    containerNames: raw.container_names ?? [],
    cpu: raw.cpu,
    memory: raw.memory,
    startedAt: raw.started_at,
    stoppedAt: raw.stopped_at,
    stoppedReason: raw.stopped_reason,
    containers: (raw.containers ?? []).map((c) => ({
      name: c.name,
      image: c.image,
      lastStatus: c.last_status,
      healthStatus: c.health_status,
      exitCode: c.exit_code,
      reason: c.reason,
    })),
  };
}

export function ecsContainerFromRaw(raw: ECSContainerRaw): ECSContainerRow {
  return {
    name: raw.name,
    runtimeId: raw.runtime_id,
    lastStatus: raw.last_status,
    execEnabled: raw.exec_enabled,
  };
}

export function ecrFromRaw(raw: ECRRepoRaw, region: string): ECRRepoRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    uri: raw.uri,
    createdAt: raw.created_at,
    imageTagMutability: raw.image_tag_mutability,
    scanOnPush: raw.scan_on_push,
  };
}

export function ecrImageFromRaw(raw: ECRImageRaw): ECRImageRow {
  return {
    id: `${raw.repository_name}@${raw.image_digest}`,
    name: raw.image_tag,
    repositoryName: raw.repository_name,
    imageTag: raw.image_tag,
    imageDigest: raw.image_digest,
    pushedAt: raw.pushed_at,
    lastPulledAt: raw.last_pulled_at,
    imageSizeBytes: raw.image_size_bytes,
  };
}

export function cfnFromRaw(raw: CFNStackRaw, region: string): CFNStackRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    createdAt: raw.creation_time,
    updatedAt: raw.last_updated_time,
    driftStatus: raw.drift_status,
    tags: raw.tags,
  };
}

export function cfnStackDetailFromRaw(raw: CFNStackDetailRaw): CFNStackDetailRow {
  return {
    stackName: raw.stack_name,
    status: raw.status,
    driftStatus: raw.drift_status,
    createdAt: raw.created_time,
    updatedAt: raw.updated_time,
    description: raw.description,
    parameters: raw.parameters.map((p) => ({
      key: p.key,
      value: p.value,
      resolvedValue: p.resolved_value,
    })),
    outputs: raw.outputs.map((o) => ({
      key: o.key,
      value: o.value,
      exportName: o.export_name,
      description: o.description,
    })),
    tags: raw.tags,
  };
}

export function cfnStackEventFromRaw(raw: CFNStackEventRaw, idx: number): CFNStackEventRow {
  return {
    id: `${raw.timestamp}-${raw.logical_resource_id}-${idx}`,
    timestamp: raw.timestamp,
    logicalResourceId: raw.logical_resource_id,
    resourceType: raw.resource_type,
    resourceStatus: raw.resource_status,
    resourceStatusReason: raw.resource_status_reason,
  };
}

export function cfnStackResourceFromRaw(raw: CFNStackResourceRaw): CFNStackResourceRow {
  return {
    id: raw.logical_resource_id,
    logicalResourceId: raw.logical_resource_id,
    physicalResourceId: raw.physical_resource_id,
    resourceType: raw.resource_type,
    resourceStatus: raw.resource_status,
    lastUpdatedTime: raw.last_updated_time,
  };
}

export function ssmFromRaw(raw: SSMParamRaw, region: string): SSMParamRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    type: raw.type,
    tier: raw.tier,
    version: raw.version,
    lastModified: raw.last_modified,
    value: raw.value,
  };
}

export function secretFromRaw(raw: SecretRaw, region: string): SecretRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    description: raw.description,
    lastChanged: raw.last_changed,
    value: raw.value,
  };
}

export function s3FromRaw(raw: S3Raw, _region: string): S3Row {
  // S3 は raw.region を優先する (バケット固有のリージョンが入るため)
  return {
    region: raw.region || _region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    createdAt: raw.created_at,
    public: raw.public,
    encryption: raw.encryption,
  };
}

export function s3ObjectFromRaw(raw: S3ObjectRaw): S3ObjectRow {
  return {
    key: raw.key,
    size: raw.size,
    lastModified: raw.last_modified,
    storageClass: raw.storage_class,
    etag: raw.etag,
  };
}

export function iamFromRaw(raw: IAMRaw, _region: string): IAMRow {
  return {
    region: 'global',
    id: raw.id,
    name: raw.name,
    state: raw.state,
    arn: raw.arn,
    kind: raw.kind,
    mfaEnabled: raw.mfa_enabled,
    lastActivity: raw.last_activity,
    groups: raw.groups ?? [],
    policies: raw.policies ?? [],
  };
}

export function elbFromRaw(raw: ELBRaw, region: string): ELBRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    type: raw.type,
    scheme: raw.scheme,
    dnsName: raw.dns_name,
    vpcId: raw.vpc_id,
    azs: raw.azs ?? [],
  };
}

export function elbListenerFromRaw(raw: ELBListenerRaw): ELBListenerRow {
  return {
    arn: raw.arn,
    loadBalancerArn: raw.load_balancer_arn,
    protocol: raw.protocol,
    port: raw.port,
    defaultActionType: raw.default_action_type,
    defaultTargetGroupArn: raw.default_target_group_arn,
  };
}

export function elbRuleFromRaw(raw: ELBRuleRaw): ELBRuleRow {
  return {
    arn: raw.arn,
    priority: raw.priority,
    isDefault: raw.is_default,
    conditions: raw.conditions ?? [],
    actionType: raw.action_type,
    targetGroupArn: raw.target_group_arn,
  };
}

export function elbTargetGroupFromRaw(raw: ELBTargetGroupRaw): ELBTargetGroupRow {
  return {
    arn: raw.arn,
    name: raw.name,
    protocol: raw.protocol,
    port: raw.port,
    targetType: raw.target_type,
    vpcId: raw.vpc_id,
    healthCheckPath: raw.health_check_path,
    loadBalancerArns: raw.load_balancer_arns ?? [],
  };
}

export function elbTargetHealthFromRaw(raw: ELBTargetHealthRaw): ELBTargetHealthRow {
  return {
    targetId: raw.target_id,
    port: raw.port,
    availabilityZone: raw.availability_zone,
    state: raw.state,
    reason: raw.reason,
    description: raw.description,
  };
}

export function cloudfrontFromRaw(raw: CloudFrontRaw, _region: string): CloudFrontRow {
  return {
    region: 'global',
    id: raw.id,
    name: raw.name,
    state: raw.state,
    domainName: raw.domain_name,
    origins: raw.origins ?? [],
    enabled: raw.enabled,
    priceClass: raw.price_class,
  };
}

export function apigwFromRaw(raw: APIGWRaw, region: string): APIGWRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    type: raw.type,
    stage: raw.stage,
    endpoint: raw.endpoint,
    tags: raw.tags ?? {},
  };
}

export function natgwFromRaw(raw: NATGWRaw, region: string): NATGWRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    vpcId: raw.vpc_id,
    subnetId: raw.subnet_id,
    elasticIp: raw.elastic_ip,
    tags: raw.tags ?? {},
    uptime: uptimeOrUndef(raw.launch_time),
    launched: launchedDate(raw.launch_time),
  };
}

export function sqsFromRaw(raw: SQSRaw, region: string): SQSRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    type: raw.type,
    availableMessages: raw.available_messages,
    inFlight: raw.in_flight,
    retentionDays: raw.retention_days,
    tags: raw.tags ?? {},
  };
}

export function kinesisFromRaw(raw: KinesisRaw, region: string): KinesisRow {
  return {
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    shardCount: raw.shard_count,
    retentionHours: raw.retention_hours,
    encryptionType: raw.encryption_type,
    tags: raw.tags ?? {},
  };
}

export function wafFromRaw(raw: WAFRaw, region: string): WAFRow {
  return {
    // WAF は CLOUDFRONT スコープの場合 global 相当だが、raw.scope が持つのでそのままリージョンを引き継ぐ
    region,
    id: raw.id,
    name: raw.name,
    state: raw.state,
    scope: raw.scope,
    ruleCount: raw.rule_count,
    associatedCount: raw.associated_count,
    tags: raw.tags ?? {},
  };
}
