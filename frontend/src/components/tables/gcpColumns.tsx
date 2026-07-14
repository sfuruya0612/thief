// GCP サービス用の列定義
import type {
  CloudRunResourceRow,
  GcsBucketRow,
  GcsObjectRow,
  IAMBindingRow,
  ServiceAccountRow,
} from '../../types/gcp';
import type { ColumnDef } from './columns';
import { formatBytes } from './columns';
import { StatusBadge } from '../primitives';

const mutedMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-2)' } as const;
const dashStyle = { color: 'var(--text-4)' } as const;

function Dash() {
  return <span style={dashStyle}>—</span>;
}

// ============================================================
// Cloud Run
// ============================================================
export const cloudRunColumns: ColumnDef<CloudRunResourceRow>[] = [
  {
    key: 'name',
    header: 'Name',
    width: '26%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'kind',
    header: 'Kind',
    width: '10%',
    cell: (r) => <StatusBadge state={r.kind} />,
  },
  {
    key: 'region',
    header: 'Region',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'uri',
    header: 'URI',
    width: '30%',
    cell: (r) => (r.uri ? <span style={mutedMono}>{r.uri}</span> : <Dash />),
  },
  {
    key: 'updateTime',
    header: 'Updated',
    width: '20%',
    cell: (r) => (r.updateTime ? <span style={mutedMono}>{r.updateTime}</span> : <Dash />),
  },
];

// ============================================================
// Cloud Storage (GCS)
// ============================================================
export const gcsBucketColumns: ColumnDef<GcsBucketRow>[] = [
  {
    key: 'name',
    header: 'Bucket',
    width: '34%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'location',
    header: 'Location',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.location}</span>,
  },
  {
    key: 'storageClass',
    header: 'Storage class',
    width: '20%',
    cell: (r) => <span style={mutedMono}>{r.storageClass}</span>,
  },
  {
    key: 'createTime',
    header: 'Created',
    width: '28%',
    cell: (r) => (r.createTime ? <span style={mutedMono}>{r.createTime}</span> : <Dash />),
  },
];

export const gcsObjectColumns: ColumnDef<GcsObjectRow>[] = [
  {
    key: 'name',
    header: 'Object',
    width: '38%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'size',
    header: 'Size',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{formatBytes(r.size)}</span>,
  },
  {
    key: 'contentType',
    header: 'Content type',
    width: '18%',
    cell: (r) => (r.contentType ? <span style={mutedMono}>{r.contentType}</span> : <Dash />),
  },
  {
    key: 'storageClass',
    header: 'Storage class',
    width: '14%',
    cell: (r) => (r.storageClass ? <span style={mutedMono}>{r.storageClass}</span> : <Dash />),
  },
  {
    key: 'updated',
    header: 'Updated',
    width: '16%',
    cell: (r) => (r.updated ? <span style={mutedMono}>{r.updated}</span> : <Dash />),
  },
];

// ============================================================
// IAM (メンバー単位に展開したバインディング)
// ============================================================
export const iamBindingColumns: ColumnDef<IAMBindingRow>[] = [
  {
    key: 'member',
    header: 'Member',
    width: '38%',
    cell: (r) => <span className="primary truncate">{r.member}</span>,
  },
  {
    key: 'role',
    header: 'Role',
    width: '32%',
    cell: (r) => <span style={mutedMono}>{r.role}</span>,
  },
  {
    key: 'conditionTitle',
    header: 'Condition',
    width: '30%',
    cell: (r) => (r.conditionTitle ? <span style={mutedMono}>{r.conditionTitle}</span> : <Dash />),
  },
];

// ============================================================
// Service Account
// ============================================================
export const serviceAccountColumns: ColumnDef<ServiceAccountRow>[] = [
  {
    key: 'email',
    header: 'Email',
    width: '36%',
    cell: (r) => <span className="primary truncate">{r.email}</span>,
  },
  {
    key: 'displayName',
    header: 'Display name',
    width: '24%',
    cell: (r) => (r.displayName ? <span style={mutedMono}>{r.displayName}</span> : <Dash />),
  },
  {
    key: 'disabled',
    header: 'Status',
    width: '14%',
    cell: (r) => <StatusBadge state={r.disabled ? 'disabled' : 'enabled'} />,
  },
  {
    key: 'description',
    header: 'Description',
    width: '26%',
    cell: (r) => (r.description ? <span style={mutedMono}>{r.description}</span> : <Dash />),
  },
];
