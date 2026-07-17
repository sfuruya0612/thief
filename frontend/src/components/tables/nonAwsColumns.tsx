// 非 AWS サービス (Datadog / TiDB) 用の列定義
// columns.tsx と同じ ColumnDef パターンを再利用する。$/mo 相当の列は対象サービスに存在しない。
// BigQuery はクエリエディタ化に伴い一覧テーブルを持たないため、列定義はここに無い。
import type { TiDBClusterRow, TiDBProjectRow } from '../../types/nonaws';
import type { ColumnDef } from './columns';
import { StatusBadge } from '../primitives';

const monoStyle = { fontFamily: 'var(--font-mono)' } as const;
const mutedMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-2)' } as const;
const dashStyle = { color: 'var(--text-4)' } as const;

function Dash() {
  return <span style={dashStyle}>—</span>;
}

// ============================================================
// TiDB
// ============================================================
export const tidbProjectColumns: ColumnDef<TiDBProjectRow>[] = [
  {
    key: 'name',
    header: 'Project',
    width: '28%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'clusterCount',
    header: 'Clusters',
    width: '16%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.clusterCount}</span>,
  },
  {
    key: 'userCount',
    header: 'Users',
    width: '16%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.userCount}</span>,
  },
  {
    key: 'orgId',
    header: 'Org ID',
    width: '20%',
    cell: (r) => <span style={mutedMono}>{r.orgId}</span>,
  },
  {
    key: 'createdAt',
    header: 'Created',
    width: '20%',
    cell: (r) => (r.createdAt ? <span style={mutedMono}>{r.createdAt}</span> : <Dash />),
  },
];

export const tidbClusterColumns: ColumnDef<TiDBClusterRow>[] = [
  {
    key: 'name',
    header: 'Cluster',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  { key: 'status', header: 'Status', width: '14%', cell: (r) => <StatusBadge state={r.status} /> },
  {
    key: 'clusterType',
    header: 'Type',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.clusterType}</span>,
  },
  {
    key: 'cloudProvider',
    header: 'Cloud',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.cloudProvider}</span>,
  },
  {
    key: 'region',
    header: 'Region',
    width: '16%',
    cell: (r) => <span style={mutedMono}>{r.region}</span>,
  },
  {
    key: 'createdAt',
    header: 'Created',
    width: '18%',
    cell: (r) => (r.createdAt ? <span style={mutedMono}>{r.createdAt}</span> : <Dash />),
  },
];
