// 非 AWS サービス (BigQuery / Datadog / TiDB) 用の列定義
// columns.tsx と同じ ColumnDef パターンを再利用する。$/mo 相当の列は対象サービスに存在しない。
import type {
  BQDatasetRow,
  BQFieldRow,
  BQTableRow,
  DatadogCostRow,
  TiDBClusterRow,
  TiDBCostRow,
  TiDBProjectRow,
} from '../../types/nonaws';
import type { ColumnDef } from './columns';
import { formatBytes } from './columns';
import { StatusBadge } from '../primitives';

const monoStyle = { fontFamily: 'var(--font-mono)' } as const;
const mutedMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-2)' } as const;
const dashStyle = { color: 'var(--text-4)' } as const;

function Dash() {
  return <span style={dashStyle}>—</span>;
}

// ============================================================
// BigQuery
// ============================================================
export const bqDatasetColumns: ColumnDef<BQDatasetRow>[] = [
  {
    key: 'name',
    header: 'Dataset',
    width: '24%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'location',
    header: 'Location',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.location}</span>,
  },
  {
    key: 'creationTime',
    header: 'Created',
    width: '18%',
    cell: (r) => (r.creationTime ? <span style={mutedMono}>{r.creationTime}</span> : <Dash />),
  },
  {
    key: 'lastModifiedTime',
    header: 'Last modified',
    width: '18%',
    cell: (r) =>
      r.lastModifiedTime ? <span style={mutedMono}>{r.lastModifiedTime}</span> : <Dash />,
  },
  {
    key: 'description',
    header: 'Description',
    width: '26%',
    cell: (r) => (r.description ? <span className="truncate">{r.description}</span> : <Dash />),
  },
];

export const bqTableColumns: ColumnDef<BQTableRow>[] = [
  {
    key: 'name',
    header: 'Table',
    width: '26%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'type',
    header: 'Type',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.type}</span>,
  },
  {
    key: 'numRows',
    header: 'Rows',
    width: '16%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{r.numRows.toLocaleString()}</span>,
  },
  {
    key: 'numBytes',
    header: 'Size',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{formatBytes(r.numBytes)}</span>,
  },
  {
    key: 'creationTime',
    header: 'Created',
    width: '16%',
    cell: (r) => (r.creationTime ? <span style={mutedMono}>{r.creationTime}</span> : <Dash />),
  },
  {
    key: 'lastModifiedTime',
    header: 'Last modified',
    width: '16%',
    cell: (r) =>
      r.lastModifiedTime ? <span style={mutedMono}>{r.lastModifiedTime}</span> : <Dash />,
  },
];

export const bqFieldColumns: ColumnDef<BQFieldRow>[] = [
  {
    key: 'name',
    header: 'Field',
    width: '28%',
    cell: (r) => <span className="primary truncate">{r.name}</span>,
  },
  {
    key: 'type',
    header: 'Type',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.type}</span>,
  },
  {
    key: 'mode',
    header: 'Mode',
    width: '18%',
    cell: (r) => <span style={mutedMono}>{r.mode}</span>,
  },
  {
    key: 'description',
    header: 'Description',
    width: '36%',
    cell: (r) => (r.description ? <span className="truncate">{r.description}</span> : <Dash />),
  },
];

// ============================================================
// Datadog
// ============================================================
export const datadogCostColumns: ColumnDef<DatadogCostRow>[] = [
  {
    key: 'month',
    header: 'Month',
    width: '12%',
    cell: (r) => <span style={mutedMono}>{r.month}</span>,
  },
  {
    key: 'orgName',
    header: 'Org',
    width: '18%',
    cell: (r) => <span className="truncate">{r.orgName}</span>,
  },
  {
    key: 'accountName',
    header: 'Account',
    width: '18%',
    cell: (r) => <span className="truncate">{r.accountName}</span>,
  },
  {
    key: 'productName',
    header: 'Product',
    width: '20%',
    cell: (r) => <span className="truncate">{r.productName}</span>,
  },
  {
    key: 'chargeType',
    header: 'Charge type',
    width: '16%',
    cell: (r) => <span style={mutedMono}>{r.chargeType}</span>,
  },
  {
    key: 'cost',
    header: 'Cost',
    width: '16%',
    align: 'right',
    cell: (r) => (
      <span style={monoStyle}>
        ${r.cost.toLocaleString(undefined, { maximumFractionDigits: 2 })}
      </span>
    ),
  },
];

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

function money(v: number): string {
  return `$${v.toLocaleString(undefined, { maximumFractionDigits: 2 })}`;
}

export const tidbCostColumns: ColumnDef<TiDBCostRow>[] = [
  {
    key: 'billedDate',
    header: 'Billed',
    width: '14%',
    cell: (r) => <span style={mutedMono}>{r.billedDate}</span>,
  },
  {
    key: 'projectName',
    header: 'Project',
    width: '18%',
    cell: (r) => (r.projectName ? <span className="truncate">{r.projectName}</span> : <Dash />),
  },
  {
    key: 'clusterName',
    header: 'Cluster',
    width: '18%',
    cell: (r) => (r.clusterName ? <span className="truncate">{r.clusterName}</span> : <Dash />),
  },
  {
    key: 'servicePathName',
    header: 'Service',
    width: '16%',
    cell: (r) =>
      r.servicePathName ? <span style={mutedMono}>{r.servicePathName}</span> : <Dash />,
  },
  {
    key: 'credits',
    header: 'Credits',
    width: '10%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{money(r.credits)}</span>,
  },
  {
    key: 'discounts',
    header: 'Discounts',
    width: '10%',
    align: 'right',
    cell: (r) => <span style={mutedMono}>{money(r.discounts)}</span>,
  },
  {
    key: 'totalCost',
    header: 'Total',
    width: '14%',
    align: 'right',
    cell: (r) => <span style={monoStyle}>{money(r.totalCost)}</span>,
  },
];
