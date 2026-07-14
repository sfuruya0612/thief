// drawer.jsx DrawerOverview の行データ生成をサービスごとの型安全な関数に分割したもの
import type { ReactNode } from 'react';
import type {
  APIGWRow,
  CacheRow,
  CloudFrontRow,
  DynamoRow,
  EC2Row,
  ECRRepoRow,
  ECSRow,
  ELBRow,
  IAMRow,
  KinesisRow,
  LambdaRow,
  NATGWRow,
  RDSRow,
  S3Row,
  SecretRow,
  SQSRow,
  SSMParamRow,
  WAFRow,
} from '../../types/aws';
import type {
  CloudRunResourceRow,
  GcsBucketRow,
  IAMMemberRow,
  ServiceAccountRow,
} from '../../types/gcp';
import { formatBytes } from '../tables/columns';

export type OverviewEntry = [string, ReactNode];

const dash = '—';

export function ec2OverviewRows(r: EC2Row): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Type', r.instanceType],
    ['AZ', r.az],
    ['Region', r.region],
    ['Private IP', r.privateIp || dash],
    ['Public IP', r.publicIp || dash],
    ['VPC', r.vpcId],
    ['Uptime', r.uptime ?? dash],
    ['Launched', r.launched ?? dash],
  ];
}

export function rdsOverviewRows(r: RDSRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Engine', r.engine],
    ['Engine version', r.engineVersion],
    ['Class', r.class],
    ['Multi-AZ', r.multiAz ? 'yes' : 'no'],
    ['Endpoint', r.endpoint],
    ['Port', r.port],
    ['VPC', r.vpcId],
    ['Region', r.region],
    ['Uptime', r.uptime ?? dash],
    ['Launched', r.launched ?? dash],
  ];
}

export function dynamoOverviewRows(r: DynamoRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Capacity mode', r.mode],
    ['Item count', r.itemCount.toLocaleString()],
    ['Size', formatBytes(r.sizeBytes)],
    ['Global secondary indexes', r.gsiCount],
    ['Region', r.region],
  ];
}

export function cacheOverviewRows(r: CacheRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Engine', r.engine],
    ['Engine version', r.engineVersion],
    ['Node type', r.nodeType],
    ['Nodes', r.numNodes],
    ['Endpoint', r.endpoint],
    ['Port', r.port],
    ['Region', r.region],
  ];
}

export function lambdaOverviewRows(r: LambdaRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Runtime', r.runtime],
    ['Memory', `${r.memoryMb} MB`],
    ['Timeout', `${r.timeoutSec}s`],
    ['Handler', r.handler],
    ['Role', r.role],
    ['Region', r.region],
  ];
}

export function ecsOverviewRows(r: ECSRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Active services', r.activeServices],
    ['Running tasks', r.runningTasks],
    ['Pending tasks', r.pendingTasks],
    ['Registered EC2', r.registeredEc2],
    ['Region', r.region],
  ];
}

export function ecrOverviewRows(r: ECRRepoRow): OverviewEntry[] {
  return [
    ['Repository ARN', r.id],
    ['URI', r.uri],
    ['Tag mutability', r.imageTagMutability],
    ['Scan on push', r.scanOnPush ? 'enabled' : 'disabled'],
    ['Created', r.createdAt || dash],
    ['Region', r.region],
  ];
}

export function ssmOverviewRows(r: SSMParamRow): OverviewEntry[] {
  return [
    ['Name', r.name],
    ['Type', r.type],
    ['Tier', r.tier],
    ['Value', r.value || dash],
    ['Version', r.version],
    ['Last modified', r.lastModified || dash],
    ['Region', r.region],
  ];
}

export function secretOverviewRows(r: SecretRow): OverviewEntry[] {
  return [
    ['Name', r.name],
    ['Value', r.value || dash],
    ['Description', r.description || dash],
    ['Last changed', r.lastChanged || dash],
    ['Region', r.region],
  ];
}

export function s3OverviewRows(r: S3Row): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Region', r.region],
    ['Created', r.createdAt || dash],
    ['Public access', r.public ? 'allowed' : 'blocked'],
    ['Encryption', r.encryption || dash],
  ];
}

export function iamOverviewRows(r: IAMRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['ARN', r.arn],
    ['Kind', r.kind],
    ['MFA', r.mfaEnabled ? 'enabled' : 'disabled'],
    ['Last active', r.lastActivity || dash],
    ['Policies attached', r.policies.length],
    ['Groups', r.groups.length],
  ];
}

export function elbOverviewRows(r: ELBRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Type', r.type],
    ['Scheme', r.scheme],
    ['DNS name', r.dnsName],
    ['VPC', r.vpcId],
    ['AZs', r.azs.join(', ') || dash],
    ['Region', r.region],
  ];
}

export function cloudfrontOverviewRows(r: CloudFrontRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Domain', r.domainName],
    ['Alternate domains', r.name || dash],
    ['Origins', r.origins.join(', ') || dash],
    ['Enabled', r.enabled ? 'yes' : 'no'],
    ['Price class', r.priceClass],
  ];
}

export function apigwOverviewRows(r: APIGWRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Type', r.type],
    ['Stage', r.stage],
    ['Endpoint', r.endpoint],
    ['Region', r.region],
  ];
}

export function natgwOverviewRows(r: NATGWRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['VPC', r.vpcId],
    ['Subnet', r.subnetId],
    ['Elastic IP', r.elasticIp],
    ['Region', r.region],
    ['Uptime', r.uptime ?? dash],
    ['Launched', r.launched ?? dash],
  ];
}

export function sqsOverviewRows(r: SQSRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Type', r.type],
    ['Messages available', r.availableMessages.toLocaleString()],
    ['Messages in flight', r.inFlight],
    ['Retention', `${r.retentionDays} days`],
    ['Region', r.region],
  ];
}

export function kinesisOverviewRows(r: KinesisRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Shards', r.shardCount],
    ['Retention', `${r.retentionHours}h`],
    ['Encryption', r.encryptionType],
    ['Region', r.region],
  ];
}

export function wafOverviewRows(r: WAFRow): OverviewEntry[] {
  return [
    ['Resource ID', r.id],
    ['Scope', r.scope],
    ['Rules', r.ruleCount],
    ['Associated resources', r.associatedCount],
    ['Region', r.region],
  ];
}

export function cloudRunOverviewRows(r: CloudRunResourceRow): OverviewEntry[] {
  return [
    ['Name', r.name],
    ['Kind', r.kind],
    ['Region', r.region],
    ['Project', r.projectId],
    ['URI', r.uri || dash],
    ['Created', r.createTime || dash],
    ['Updated', r.updateTime || dash],
  ];
}

export function gcsBucketOverviewRows(r: GcsBucketRow): OverviewEntry[] {
  return [
    ['Bucket', r.name],
    ['Location', r.location],
    ['Storage class', r.storageClass],
    ['Created', r.createTime || dash],
  ];
}

export function iamMemberOverviewRows(r: IAMMemberRow): OverviewEntry[] {
  return [
    ['Member', r.member],
    ['Project', r.projectId],
    ['Roles', r.roles.length],
    ...r.roles.map((role, i): OverviewEntry => [`Role ${i + 1}`, role]),
  ];
}

export function serviceAccountOverviewRows(r: ServiceAccountRow): OverviewEntry[] {
  return [
    ['Email', r.email],
    ['Display name', r.displayName || dash],
    ['Description', r.description || dash],
    ['Project', r.projectId],
    ['Unique ID', r.uniqueId],
    ['Status', r.disabled ? 'disabled' : 'enabled'],
  ];
}
