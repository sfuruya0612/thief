// tables.jsx COLS の型安全な移植
// モックは全サービス共通のフラットなオブジェクトを使うが、実際の XxxRow 型は
// サービスごとに異なるフィールド名を持つため、サービスごとに個別の列定義を書く。
// $/mo (cost) 列は確定方針により全サービスで削除する。
import type { ReactNode } from 'react';
import type {
  APIGWRow,
  CacheParameterRow,
  CacheRow,
  CFNStackEventRow,
  CFNStackResourceRow,
  CFNStackRow,
  CloudFrontRow,
  DynamoRow,
  EC2Row,
  ECRImageRow,
  ECRRepoRow,
  ECSRow,
  ECSServiceRow,
  ECSTaskRow,
  ELBListenerRow,
  ELBRow,
  ELBRuleRow,
  ELBTargetGroupRow,
  ELBTargetHealthRow,
  IAMRow,
  KinesisRow,
  LambdaRow,
  NATGWRow,
  RDSParameterRow,
  RDSRow,
  S3ObjectRow,
  S3Row,
  SecretRow,
  SQSRow,
  SSMParamRow,
  WAFRow,
} from '../../types/aws';
import { StatusBadge } from '../primitives';

export interface ColumnDef<T> {
  key: string;
  header: string;
  width: string;
  align?: 'right';
  cell: (row: T) => ReactNode;
  // 列フィルターを明示的に有効/無効化する。未指定時は header が空 or key === 'actions' 以外を対象とする
  filterable?: boolean;
  // フィルター判定に使う文字列を返す。表示値でフィルタしたい列 (例: size は formatBytes 後の値) で指定する。
  // 未指定時は row[key] の生値を String() 化して用いる
  filterValue?: (row: T) => string;
}

// バイト数を人が読みやすい単位 (KB/MB/GB/TB) に変換する
export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

const monoStyle = { fontFamily: 'var(--font-mono)' } as const;
const mutedMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-2)' } as const;
const dimMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-3)' } as const;
const dashStyle = { color: 'var(--text-4)' } as const;

function Dash() {
  return <span style={dashStyle}>—</span>;
}

// ============================================================
// EC2
// ============================================================
export const ec2Columns: ColumnDef<EC2Row>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '20%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'instanceType',
    header: 'Type',
    width: '11%',
    cell: (r) => <span style={monoStyle}>{r.instanceType}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '13%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'az',
    header: 'AZ',
    width: '7%',
    cell: (r) => <span style={dimMono}>{r.az.slice(-1)}</span>,
  },
  {
    key: 'privateIp',
    header: 'Private IP',
    width: '13%',
    cell: (r) => <span style={mutedMono}>{r.privateIp}</span>,
  },
  {
    key: 'publicIp',
    header: 'Public IP',
    width: '13%',
    cell: (r) => (r.publicIp ? <span style={mutedMono}>{r.publicIp}</span> : <Dash />),
  },
  {
    key: 'uptime',
    header: 'Uptime',
    width: '13%',
    cell: (r) => (r.uptime ? <span style={mutedMono}>{r.uptime}</span> : <Dash />),
  },
];

// ============================================================
// RDS
// ============================================================
export const rdsColumns: ColumnDef<RDSRow>[] = [
  {
    key: 'name',
    header: 'Identifier',
    width: '14%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '8%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'engine',
    header: 'Engine',
    width: '9%',
    cell: (r) => <span style={mutedMono}>{r.engine}</span>,
  },
  {
    key: 'engineVersion',
    header: 'Engine Version',
    width: '10%',
    cell: (r) => <span style={mutedMono}>{r.engineVersion}</span>,
  },
  {
    key: 'clusterId',
    header: 'Cluster',
    width: '9%',
    cell: (r) => (r.clusterId ? <span style={mutedMono}>{r.clusterId}</span> : <Dash />),
  },
  {
    key: 'class',
    header: 'Class',
    width: '9%',
    cell: (r) => <span style={monoStyle}>{r.class}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '9%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'multiAz',
    header: 'MultiAZ',
    width: '6%',
    cell: (r) => (r.multiAz ? <span style={{ color: 'var(--ok)' }}>✓</span> : <Dash />),
  },
  {
    key: 'endpoint',
    header: 'Endpoint',
    width: '16%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.endpoint}
      </span>
    ),
  },
  {
    key: 'uptime',
    header: 'Uptime',
    width: '10%',
    cell: (r) => (r.uptime ? <span style={mutedMono}>{r.uptime}</span> : <Dash />),
  },
];

// ============================================================
// RDS / ElastiCache パラメータグループ (Drawer の Parameters タブのサブリソース列)
// ============================================================
export const rdsParameterColumns: ColumnDef<RDSParameterRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '26%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'value',
    header: 'Value',
    width: '22%',
    cell: (r) =>
      r.value ? (
        <span
          className="truncate"
          style={{ ...monoStyle, display: 'inline-block', maxWidth: '100%' }}
          title={r.value}
        >
          {r.value}
        </span>
      ) : (
        <Dash />
      ),
  },
  {
    key: 'applyType',
    header: 'Apply type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.applyType}</span>,
  },
  {
    key: 'dataType',
    header: 'Data type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.dataType}</span>,
  },
  {
    key: 'isModifiable',
    header: 'Modifiable',
    width: '10%',
    cell: (r) => (r.isModifiable ? <span style={{ color: 'var(--ok)' }}>✓</span> : <Dash />),
  },
  {
    key: 'source',
    header: 'Source',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.source}</span>,
  },
];

export const cacheParameterColumns: ColumnDef<CacheParameterRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '26%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'value',
    header: 'Value',
    width: '22%',
    cell: (r) =>
      r.value ? (
        <span
          className="truncate"
          style={{ ...monoStyle, display: 'inline-block', maxWidth: '100%' }}
          title={r.value}
        >
          {r.value}
        </span>
      ) : (
        <Dash />
      ),
  },
  {
    key: 'changeType',
    header: 'Change type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.changeType}</span>,
  },
  {
    key: 'dataType',
    header: 'Data type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.dataType}</span>,
  },
  {
    key: 'isModifiable',
    header: 'Modifiable',
    width: '10%',
    cell: (r) => (r.isModifiable ? <span style={{ color: 'var(--ok)' }}>✓</span> : <Dash />),
  },
  {
    key: 'source',
    header: 'Source',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.source}</span>,
  },
];

// ============================================================
// ElastiCache (cache)
// ============================================================
export const cacheColumns: ColumnDef<CacheRow>[] = [
  {
    key: 'name',
    header: 'Cluster',
    width: '18%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'engine',
    header: 'Engine',
    width: '13%',
    cell: (r) => <span style={mutedMono}>{r.engine}</span>,
  },
  {
    key: 'nodeType',
    header: 'Node type',
    width: '14%',
    cell: (r) => <span style={monoStyle}>{r.nodeType}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '13%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'numNodes',
    header: 'Nodes',
    width: '8%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.numNodes}</span>,
  },
  {
    key: 'endpoint',
    header: 'Endpoint',
    width: '24%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.endpoint}
      </span>
    ),
  },
];

// ============================================================
// Lambda
// ============================================================
export const lambdaColumns: ColumnDef<LambdaRow>[] = [
  {
    key: 'name',
    header: 'Function',
    width: '30%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '12%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'runtime',
    header: 'Runtime',
    width: '16%',
    cell: (r) => <span style={mutedMono}>{r.runtime}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '16%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'memoryMb',
    header: 'Memory',
    width: '13%',
    cell: (r) => <span style={monoStyle}>{r.memoryMb} MB</span>,
  },
  {
    key: 'timeoutSec',
    header: 'Timeout',
    width: '13%',
    cell: (r) => <span style={dimMono}>{r.timeoutSec}s</span>,
  },
];

// ============================================================
// ECS
// バックエンドの ECSResource はクラスタ単位の集計値であり、モックのタスク単位の
// データ構造とは異なるため、実際に利用可能なフィールド (activeServices 等) で構成する
// ============================================================
export const ecsColumns: ColumnDef<ECSRow>[] = [
  {
    key: 'name',
    header: 'Cluster',
    width: '20%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'activeServices',
    header: 'Active svc',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.activeServices}</span>,
  },
  {
    key: 'runningTasks',
    header: 'Running',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.runningTasks}</span>,
  },
  {
    key: 'pendingTasks',
    header: 'Pending',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.pendingTasks}</span>,
  },
  {
    key: 'registeredEc2',
    header: 'Registered EC2',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.registeredEc2}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
];

// ============================================================
// ECS Services / Tasks (Drawer の Services / Tasks タブ)
// ============================================================
export const ecsServiceColumns: ColumnDef<ECSServiceRow>[] = [
  {
    key: 'name',
    header: 'Service',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'status', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.status} /> },
  {
    key: 'desiredCount',
    header: 'Desired',
    width: '12%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.desiredCount}</span>,
  },
  {
    key: 'runningCount',
    header: 'Running',
    width: '12%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.runningCount}</span>,
  },
  {
    key: 'pendingCount',
    header: 'Pending',
    width: '12%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.pendingCount}</span>,
  },
  {
    key: 'launchType',
    header: 'Launch type',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.launchType}</span>,
  },
  {
    key: 'taskDefinition',
    header: 'Task definition',
    width: '16%',
    cell: (r) => <span style={dimMono}>{r.taskDefinition}</span>,
  },
];

export const ecsTaskColumns: ColumnDef<ECSTaskRow>[] = [
  {
    key: 'group',
    header: 'Group',
    width: '18%',
    cell: (r) => <span className="primary truncate">{r.group}</span>,
  },
  {
    key: 'containerNames',
    header: 'Containers',
    width: '18%',
    cell: (r) => <span className="truncate">{r.containerNames.join(', ')}</span>,
  },
  {
    key: 'lastStatus',
    header: 'Last status',
    width: '12%',
    cell: (r) => <StatusBadge state={r.lastStatus} />,
  },
  {
    key: 'desiredStatus',
    header: 'Desired status',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.desiredStatus}</span>,
  },
  {
    key: 'launchType',
    header: 'Launch type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.launchType}</span>,
  },
  {
    key: 'enableExecuteCommand',
    header: 'Exec enabled',
    width: '12%',
    cell: (r) =>
      r.enableExecuteCommand ? <span style={{ color: 'var(--ok)' }}>✓</span> : <Dash />,
  },
  {
    key: 'arn',
    header: 'ARN',
    width: '16%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.arn}
      </span>
    ),
  },
];

// ============================================================
// ECR
// ============================================================
export const ecrColumns: ColumnDef<ECRRepoRow>[] = [
  {
    key: 'name',
    header: 'Repository',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'uri',
    header: 'URI',
    width: '30%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.uri}
      </span>
    ),
  },
  {
    key: 'imageTagMutability',
    header: 'Tag mutability',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.imageTagMutability}</span>,
  },
  {
    key: 'scanOnPush',
    header: 'Scan on push',
    width: '12%',
    cell: (r) => (r.scanOnPush ? <span style={{ color: 'var(--ok)' }}>✓</span> : <Dash />),
  },
  {
    key: 'createdAt',
    header: 'Created',
    width: '20%',
    cell: (r) => (r.createdAt ? <span style={dimMono}>{r.createdAt}</span> : <Dash />),
  },
];

export const ecrImageColumns: ColumnDef<ECRImageRow>[] = [
  {
    key: 'imageTag',
    header: 'Tag',
    width: '20%',
    cell: (r) => <span className="primary truncate">{r.imageTag || <Dash />}</span>,
  },
  {
    key: 'imageDigest',
    header: 'Digest',
    width: '38%',
    cell: (r) => (
      <span
        className="truncate"
        style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}
        title={r.imageDigest}
      >
        {r.imageDigest.slice(0, 19)}…
      </span>
    ),
  },
  {
    key: 'imageSizeBytes',
    header: 'Size',
    width: '10%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{formatBytes(r.imageSizeBytes)}</span>,
  },
  {
    key: 'pushedAt',
    header: 'Pushed',
    width: '16%',
    cell: (r) => (r.pushedAt ? <span style={dimMono}>{r.pushedAt}</span> : <Dash />),
  },
  {
    key: 'lastPulledAt',
    header: 'Pulled',
    width: '16%',
    cell: (r) => (r.lastPulledAt ? <span style={dimMono}>{r.lastPulledAt}</span> : <Dash />),
  },
];

// ============================================================
// SSM Parameter Store
// ============================================================
export const ssmColumns: ColumnDef<SSMParamRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '42%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'type',
    header: 'Type',
    width: '15%',
    cell: (r) => <span style={mutedMono}>{r.type}</span>,
  },
  {
    key: 'version',
    header: 'Version',
    width: '8%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.version}</span>,
  },
  {
    key: 'lastModified',
    header: 'Last modified',
    width: '35%',
    cell: (r) => (r.lastModified ? <span style={dimMono}>{r.lastModified}</span> : <Dash />),
  },
];

// ============================================================
// Secrets Manager
// ============================================================
export const secretColumns: ColumnDef<SecretRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '42%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'description',
    header: 'Description',
    width: '33%',
    cell: (r) => <span className="truncate">{r.description || <Dash />}</span>,
  },
  {
    key: 'lastChanged',
    header: 'Last changed',
    width: '25%',
    cell: (r) => (r.lastChanged ? <span style={dimMono}>{r.lastChanged}</span> : <Dash />),
  },
];

// ============================================================
// S3
// ============================================================
export const s3Columns: ColumnDef<S3Row>[] = [
  {
    key: 'name',
    header: 'Bucket',
    width: '28%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '12%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'region',
    header: 'Region',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'createdAt',
    header: 'Created',
    width: '16%',
    cell: (r) => <span style={dimMono}>{r.createdAt}</span>,
  },
  {
    key: 'public',
    header: 'Public',
    width: '14%',
    cell: (r) =>
      r.public ? (
        <span className="status err">
          <span className="dot" />
          public
        </span>
      ) : (
        <span style={dashStyle}>private</span>
      ),
  },
  {
    key: 'encryption',
    header: 'Encryption',
    width: '16%',
    cell: (r) => <span style={mutedMono}>{r.encryption}</span>,
  },
];

export const s3ObjectColumns: ColumnDef<S3ObjectRow>[] = [
  {
    key: 'key',
    header: 'Key',
    width: '40%',
    cell: (r) => <span className="primary truncate">{r.key}</span>,
  },
  {
    key: 'size',
    header: 'Size',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{formatBytes(r.size)}</span>,
  },
  {
    key: 'lastModified',
    header: 'Last modified',
    width: '18%',
    cell: (r) => (r.lastModified ? <span style={dimMono}>{r.lastModified}</span> : <Dash />),
  },
  {
    key: 'storageClass',
    header: 'Storage class',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.storageClass}</span>,
  },
];

// ============================================================
// ELB
// ============================================================
export const elbColumns: ColumnDef<ELBRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '18%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'type',
    header: 'Type',
    width: '8%',
    cell: (r) => (
      <span className="svc-pill">
        <span className="dot" style={{ background: r.type === 'ALB' ? '#f2994a' : '#4ea7fc' }} />
        {r.type}
      </span>
    ),
  },
  {
    key: 'scheme',
    header: 'Scheme',
    width: '13%',
    cell: (r) => <span style={mutedMono}>{r.scheme}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'dnsName',
    header: 'DNS name',
    width: '24%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.dnsName}
      </span>
    ),
  },
  {
    key: 'azs',
    header: 'AZs',
    width: '15%',
    cell: (r) => <span style={mutedMono}>{r.azs.join(', ') || '—'}</span>,
  },
];

// ============================================================
// ELB Listener / Rule / TargetGroup / TargetHealth
// ============================================================
export const elbListenerColumns: ColumnDef<ELBListenerRow>[] = [
  {
    key: 'protocol',
    header: 'Protocol',
    width: '14%',
    cell: (r) => <span className="primary">{r.protocol}</span>,
  },
  {
    key: 'port',
    header: 'Port',
    width: '10%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.port}</span>,
  },
  {
    key: 'defaultActionType',
    header: 'Default action',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.defaultActionType || <Dash />}</span>,
  },
  {
    key: 'defaultTargetGroupArn',
    header: 'Default target group',
    width: '38%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.defaultTargetGroupArn || <Dash />}
      </span>
    ),
  },
  {
    key: 'arn',
    header: 'ARN',
    width: '20%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.arn}
      </span>
    ),
  },
];

export const elbRuleColumns: ColumnDef<ELBRuleRow>[] = [
  {
    key: 'priority',
    header: 'Priority',
    width: '10%',
    cell: (r) => <span className="primary">{r.isDefault ? 'default' : r.priority}</span>,
  },
  {
    key: 'conditions',
    header: 'Conditions',
    width: '32%',
    cell: (r) => <span className="truncate">{r.conditions.join(' / ') || <Dash />}</span>,
  },
  {
    key: 'actionType',
    header: 'Action',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.actionType || <Dash />}</span>,
  },
  {
    key: 'targetGroupArn',
    header: 'Target group',
    width: '28%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.targetGroupArn || <Dash />}
      </span>
    ),
  },
  {
    key: 'arn',
    header: 'ARN',
    width: '16%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.arn}
      </span>
    ),
  },
];

export const elbTargetGroupColumns: ColumnDef<ELBTargetGroupRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '22%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'protocol',
    header: 'Protocol',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.protocol}</span>,
  },
  {
    key: 'port',
    header: 'Port',
    width: '8%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.port}</span>,
  },
  {
    key: 'targetType',
    header: 'Target type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.targetType}</span>,
  },
  {
    key: 'healthCheckPath',
    header: 'Health check',
    width: '18%',
    cell: (r) => <span style={dimMono}>{r.healthCheckPath || <Dash />}</span>,
  },
  {
    key: 'vpcId',
    header: 'VPC',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.vpcId}</span>,
  },
  {
    key: 'arn',
    header: 'ARN',
    width: '14%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.arn}
      </span>
    ),
  },
];

export const elbTargetHealthColumns: ColumnDef<ELBTargetHealthRow>[] = [
  {
    key: 'targetId',
    header: 'Target',
    width: '20%',
    cell: (r) => (
      <span className="primary truncate" style={monoStyle}>
        {r.targetId}
      </span>
    ),
  },
  {
    key: 'port',
    header: 'Port',
    width: '8%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.port}</span>,
  },
  {
    key: 'state',
    header: 'State',
    width: '12%',
    cell: (r) => <StatusBadge state={r.state} />,
  },
  {
    key: 'availabilityZone',
    header: 'AZ',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.availabilityZone || <Dash />}</span>,
  },
  {
    key: 'reason',
    header: 'Reason',
    width: '20%',
    cell: (r) => <span style={dimMono}>{r.reason || <Dash />}</span>,
  },
  {
    key: 'description',
    header: 'Description',
    width: '26%',
    cell: (r) => <span className="truncate">{r.description || <Dash />}</span>,
  },
];

// ============================================================
// CloudFront (region 列は無し: グローバルサービス)
// ============================================================
export const cloudfrontColumns: ColumnDef<CloudFrontRow>[] = [
  {
    key: 'id',
    header: 'Distribution',
    width: '14%',
    cell: (r) => (
      <span className="primary truncate" style={monoStyle}>
        {r.id}
      </span>
    ),
  },
  { key: 'state', header: 'State', width: '11%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'domainName',
    header: 'Domain',
    width: '22%',
    cell: (r) => (
      <span
        className="truncate"
        style={{ ...mutedMono, display: 'inline-block', maxWidth: '100%' }}
      >
        {r.domainName}
      </span>
    ),
  },
  {
    key: 'name',
    header: 'Alternate domains',
    width: '20%',
    cell: (r) => <span className="truncate">{r.name}</span>,
  },
  {
    key: 'origins',
    header: 'Origins',
    width: '19%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.origins.join(', ') || '—'}
      </span>
    ),
  },
  {
    key: 'enabled',
    header: 'Enabled',
    width: '7%',
    cell: (r) => (r.enabled ? <span style={{ color: 'var(--ok)' }}>✓</span> : <Dash />),
  },
  {
    key: 'priceClass',
    header: 'Price class',
    width: '7%',
    cell: (r) => <span style={dimMono}>{r.priceClass}</span>,
  },
];

// ============================================================
// API Gateway
// ============================================================
export const apigwColumns: ColumnDef<APIGWRow>[] = [
  {
    key: 'name',
    header: 'API',
    width: '22%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'type',
    header: 'Type',
    width: '12%',
    cell: (r) => (
      <span className="svc-pill">
        <span
          className="dot"
          style={{
            background: r.type === 'REST' ? '#4ea7fc' : r.type === 'HTTP' ? '#4cb782' : '#de5d9c',
          }}
        />
        {r.type}
      </span>
    ),
  },
  {
    key: 'stage',
    header: 'Stage',
    width: '9%',
    cell: (r) => <span style={mutedMono}>{r.stage}</span>,
  },
  {
    key: 'endpoint',
    header: 'Endpoint',
    width: '29%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.endpoint}
      </span>
    ),
  },
  {
    key: 'region',
    header: 'Region',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
];

// ============================================================
// NAT Gateway
// ============================================================
export const natgwColumns: ColumnDef<NATGWRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '16%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '11%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'id',
    header: 'Gateway ID',
    width: '18%',
    cell: (r) => (
      <span
        className="truncate"
        style={{ ...mutedMono, display: 'inline-block', maxWidth: '100%' }}
      >
        {r.id}
      </span>
    ),
  },
  {
    key: 'vpcId',
    header: 'VPC',
    width: '13%',
    cell: (r) => <span style={dimMono}>{r.vpcId}</span>,
  },
  {
    key: 'elasticIp',
    header: 'Elastic IP',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.elasticIp}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '15%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'uptime',
    header: 'Uptime',
    width: '13%',
    cell: (r) => (r.uptime ? <span style={mutedMono}>{r.uptime}</span> : <Dash />),
  },
];

// ============================================================
// DynamoDB
// ============================================================
export const dynamoColumns: ColumnDef<DynamoRow>[] = [
  {
    key: 'name',
    header: 'Table',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'mode',
    header: 'Mode',
    width: '15%',
    cell: (r) => <span style={mutedMono}>{r.mode}</span>,
  },
  {
    key: 'itemCount',
    header: 'Items',
    width: '13%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.itemCount.toLocaleString()}</span>,
  },
  {
    key: 'sizeBytes',
    header: 'Size',
    width: '12%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{formatBytes(r.sizeBytes)}</span>,
  },
  {
    key: 'gsiCount',
    header: 'GSI',
    width: '8%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.gsiCount}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
];

// ============================================================
// SQS
// ============================================================
export const sqsColumns: ColumnDef<SQSRow>[] = [
  {
    key: 'name',
    header: 'Queue',
    width: '22%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'type',
    header: 'Type',
    width: '12%',
    cell: (r) => (
      <span className="svc-pill">
        <span className="dot" style={{ background: r.type === 'FIFO' ? '#a97ce8' : '#4cb782' }} />
        {r.type}
      </span>
    ),
  },
  {
    key: 'availableMessages',
    header: 'Available',
    width: '12%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.availableMessages.toLocaleString()}</span>,
  },
  {
    key: 'inFlight',
    header: 'In flight',
    width: '11%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.inFlight}</span>,
  },
  {
    key: 'retentionDays',
    header: 'Retention',
    width: '11%',
    cell: (r) => <span style={mutedMono}>{r.retentionDays}d</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '22%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
];

// ============================================================
// Kinesis
// ============================================================
export const kinesisColumns: ColumnDef<KinesisRow>[] = [
  {
    key: 'name',
    header: 'Stream',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '12%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'shardCount',
    header: 'Shards',
    width: '11%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.shardCount}</span>,
  },
  {
    key: 'retentionHours',
    header: 'Retention',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.retentionHours}h</span>,
  },
  {
    key: 'encryptionType',
    header: 'Encryption',
    width: '15%',
    cell: (r) => <span style={mutedMono}>{r.encryptionType}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '24%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
];

// ============================================================
// WAF
// ============================================================
export const wafColumns: ColumnDef<WAFRow>[] = [
  {
    key: 'name',
    header: 'Web ACL',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '10%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'scope',
    header: 'Scope',
    width: '15%',
    cell: (r) => (
      <span className="svc-pill">
        <span
          className="dot"
          style={{ background: r.scope === 'CLOUDFRONT' ? '#4ea7fc' : '#f2994a' }}
        />
        {r.scope}
      </span>
    ),
  },
  {
    key: 'ruleCount',
    header: 'Rules',
    width: '12%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.ruleCount}</span>,
  },
  {
    key: 'associatedCount',
    header: 'Associated',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.associatedCount}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '25%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
];

// ============================================================
// IAM (region 列は無し: グローバルサービス)
// ============================================================
export const iamColumns: ColumnDef<IAMRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '22%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'kind',
    header: 'Kind',
    width: '11%',
    cell: (r) => (
      <span className="svc-pill">
        <span className="dot" style={{ background: r.kind === 'role' ? '#a855f7' : '#ec4899' }} />
        {r.kind}
      </span>
    ),
  },
  {
    key: 'mfaEnabled',
    header: 'MFA',
    width: '9%',
    cell: (r) =>
      r.mfaEnabled ? (
        <span style={{ color: 'var(--ok)' }}>✓</span>
      ) : (
        <span style={{ color: 'var(--err)' }}>✕</span>
      ),
  },
  { key: 'state', header: 'Activity', width: '13%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'lastActivity',
    header: 'Last active',
    width: '16%',
    cell: (r) => (r.lastActivity ? <span style={mutedMono}>{r.lastActivity}</span> : <Dash />),
  },
  {
    key: 'policies',
    header: 'Policies',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.policies.length}</span>,
  },
  {
    key: 'groups',
    header: 'Groups',
    width: '15%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{r.groups.length}</span>,
  },
];

// ============================================================
// CloudFormation
// ============================================================
export const cfnColumns: ColumnDef<CFNStackRow>[] = [
  {
    key: 'name',
    header: 'Stack',
    width: '26%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'state', header: 'State', width: '16%', cell: (r) => <StatusBadge state={r.state} /> },
  {
    key: 'driftStatus',
    header: 'Drift',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.driftStatus}</span>,
  },
  {
    key: 'createdAt',
    header: 'Created',
    width: '22%',
    cell: (r) => (r.createdAt ? <span style={dimMono}>{r.createdAt}</span> : <Dash />),
  },
  {
    key: 'updatedAt',
    header: 'Updated',
    width: '22%',
    cell: (r) => (r.updatedAt ? <span style={dimMono}>{r.updatedAt}</span> : <Dash />),
  },
];

// CFN イベントの失敗系ステータス (CREATE_FAILED / ROLLBACK 系) は色分けする
const CFN_EVENT_FAILURE_RE = /FAILED|ROLLBACK/;

export function isCfnEventFailure(status: string): boolean {
  return CFN_EVENT_FAILURE_RE.test(status);
}

export const cfnEventColumns: ColumnDef<CFNStackEventRow>[] = [
  {
    key: 'timestamp',
    header: 'Time',
    width: '18%',
    cell: (r) => (r.timestamp ? <span style={dimMono}>{r.timestamp}</span> : <Dash />),
  },
  {
    key: 'logicalResourceId',
    header: 'Logical ID',
    width: '20%',
    cell: (r) => <span className="truncate">{r.logicalResourceId}</span>,
  },
  {
    key: 'resourceType',
    header: 'Type',
    width: '20%',
    cell: (r) => <span style={mutedMono}>{r.resourceType}</span>,
  },
  {
    key: 'resourceStatus',
    header: 'Status',
    width: '14%',
    cell: (r) => (
      <span style={{ color: isCfnEventFailure(r.resourceStatus) ? 'var(--err)' : undefined }}>
        {r.resourceStatus}
      </span>
    ),
  },
  {
    key: 'resourceStatusReason',
    header: 'Reason',
    width: '28%',
    cell: (r) => (
      <span className="truncate" title={r.resourceStatusReason}>
        {r.resourceStatusReason || <Dash />}
      </span>
    ),
  },
];

export const cfnResourceColumns: ColumnDef<CFNStackResourceRow>[] = [
  {
    key: 'logicalResourceId',
    header: 'Logical ID',
    width: '22%',
    cell: (r) => <span className="primary truncate">{r.logicalResourceId}</span>,
  },
  {
    key: 'physicalResourceId',
    header: 'Physical ID',
    width: '26%',
    cell: (r) => (
      <span className="truncate" style={{ ...dimMono, display: 'inline-block', maxWidth: '100%' }}>
        {r.physicalResourceId || <Dash />}
      </span>
    ),
  },
  {
    key: 'resourceType',
    header: 'Type',
    width: '22%',
    cell: (r) => <span style={mutedMono}>{r.resourceType}</span>,
  },
  {
    key: 'resourceStatus',
    header: 'Status',
    width: '14%',
    cell: (r) => <StatusBadge state={r.resourceStatus} />,
  },
  {
    key: 'lastUpdatedTime',
    header: 'Updated',
    width: '16%',
    cell: (r) => (r.lastUpdatedTime ? <span style={dimMono}>{r.lastUpdatedTime}</span> : <Dash />),
  },
];
