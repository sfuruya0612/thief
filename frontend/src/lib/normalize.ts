// Raw (バックエンド JSON) → Row (UI 用) 変換関数
import { formatUptime } from './format';
import type {
  APIGWRaw,
  APIGWRow,
  CacheRaw,
  CacheRow,
  CloudFrontRaw,
  CloudFrontRow,
  DynamoRaw,
  DynamoRow,
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
  ELBRaw,
  ELBRow,
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
    imageSizeBytes: raw.image_size_bytes,
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
