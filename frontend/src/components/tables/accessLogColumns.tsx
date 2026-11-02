// Access Logs の集計結果の列定義。DataTable の列フィルタとソートをそのまま使う。
import type { AccessLogIPRow, AccessLogPairRow } from '../../types/accesslogs';
import type { ColumnDef } from './columns';
import { Dash, monoStyle } from './cells';

function formatTime(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function flagClass(flag: string): string {
  switch (flag) {
    case 'blocked':
      return 'status err';
    case 'challenged':
      return 'status warn';
    case 'client_error':
    case 'server_error':
      return 'status info';
    default:
      return 'status muted';
  }
}

function flagLabel(flag: string): string {
  switch (flag) {
    case 'client_error':
      return '4xx';
    case 'server_error':
      return '5xx';
    case 'sensitive_path':
      return 'sensitive path';
    case 'has_query':
      return 'query';
    case 'many_uris':
      return 'many URIs';
    case 'count_matched':
      return 'COUNT';
    default:
      return flag;
  }
}

function Flags({ flags }: { flags: string[] }) {
  if (flags.length === 0) return <Dash />;
  return (
    <span className="accesslogs-flags">
      {flags.map((flag) => (
        <span key={flag} className={flagClass(flag)}>
          {flagLabel(flag)}
        </span>
      ))}
    </span>
  );
}

function Sources({ labels }: { labels: string[] }) {
  if (labels.length === 0) return <Dash />;
  return <span title={labels.join(', ')}>{labels.join(', ')}</span>;
}

export const accessLogPairColumns: ColumnDef<AccessLogPairRow>[] = [
  {
    key: 'clientIp',
    header: 'Source IP',
    width: '150px',
    cell: (row) => <span style={monoStyle}>{row.clientIp}</span>,
  },
  {
    key: 'uriPath',
    header: 'URI',
    width: '320px',
    cell: (row) => (
      <span
        style={monoStyle}
        title={row.sampleQuery ? `${row.uriPath}?${row.sampleQuery}` : row.uriPath}
      >
        {row.uriPath}
        {row.hasQuery ? <span className="accesslogs-query-mark">?</span> : null}
      </span>
    ),
  },
  {
    key: 'requestCount',
    header: 'Requests',
    width: '90px',
    align: 'right',
    cell: (row) => row.requestCount.toLocaleString(),
  },
  {
    key: 'blockedCount',
    header: 'Blocked',
    width: '80px',
    align: 'right',
    cell: (row) =>
      row.blockedCount > 0 ? <strong>{row.blockedCount.toLocaleString()}</strong> : <Dash />,
  },
  {
    key: 'challengedCount',
    header: 'Challenged',
    width: '100px',
    align: 'right',
    cell: (row) => (row.challengedCount > 0 ? row.challengedCount.toLocaleString() : <Dash />),
  },
  {
    key: 'clientErrorCount',
    header: '4xx',
    width: '70px',
    align: 'right',
    cell: (row) => (row.clientErrorCount > 0 ? row.clientErrorCount.toLocaleString() : <Dash />),
  },
  {
    key: 'serverErrorCount',
    header: '5xx',
    width: '70px',
    align: 'right',
    cell: (row) => (row.serverErrorCount > 0 ? row.serverErrorCount.toLocaleString() : <Dash />),
  },
  {
    key: 'sourceLabels',
    header: 'Sources',
    width: '130px',
    filterValue: (row) => row.sourceLabels.join(' '),
    cell: (row) => <Sources labels={row.sourceLabels} />,
  },
  {
    key: 'lastSeen',
    header: 'Last seen',
    width: '160px',
    cell: (row) => (row.lastSeen ? formatTime(row.lastSeen) : <Dash />),
  },
  {
    key: 'flags',
    header: 'Flags',
    width: '200px',
    filterValue: (row) => row.flags.join(' '),
    cell: (row) => <Flags flags={row.flags} />,
  },
];

export const accessLogIPColumns: ColumnDef<AccessLogIPRow>[] = [
  {
    key: 'clientIp',
    header: 'Source IP',
    width: '170px',
    cell: (row) => <span style={monoStyle}>{row.clientIp}</span>,
  },
  {
    key: 'requestCount',
    header: 'Requests',
    width: '100px',
    align: 'right',
    cell: (row) => row.requestCount.toLocaleString(),
  },
  {
    key: 'blockedCount',
    header: 'Blocked',
    width: '90px',
    align: 'right',
    cell: (row) =>
      row.blockedCount > 0 ? <strong>{row.blockedCount.toLocaleString()}</strong> : <Dash />,
  },
  {
    key: 'challengedCount',
    header: 'Challenged',
    width: '110px',
    align: 'right',
    cell: (row) => (row.challengedCount > 0 ? row.challengedCount.toLocaleString() : <Dash />),
  },
  {
    key: 'clientErrorCount',
    header: '4xx',
    width: '80px',
    align: 'right',
    cell: (row) => (row.clientErrorCount > 0 ? row.clientErrorCount.toLocaleString() : <Dash />),
  },
  {
    key: 'serverErrorCount',
    header: '5xx',
    width: '80px',
    align: 'right',
    cell: (row) => (row.serverErrorCount > 0 ? row.serverErrorCount.toLocaleString() : <Dash />),
  },
  {
    key: 'distinctUris',
    header: 'Distinct URIs',
    width: '110px',
    align: 'right',
    cell: (row) => row.distinctUris.toLocaleString(),
  },
  {
    key: 'sourceLabels',
    header: 'Sources',
    width: '140px',
    filterValue: (row) => row.sourceLabels.join(' '),
    cell: (row) => <Sources labels={row.sourceLabels} />,
  },
  {
    key: 'lastSeen',
    header: 'Last seen',
    width: '170px',
    cell: (row) => (row.lastSeen ? formatTime(row.lastSeen) : <Dash />),
  },
  {
    key: 'flags',
    header: 'Flags',
    width: '210px',
    filterValue: (row) => row.flags.join(' '),
    cell: (row) => <Flags flags={row.flags} />,
  },
];
