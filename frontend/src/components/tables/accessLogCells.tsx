// Access Logs の列セル。列定義 (accessLogColumns.tsx) から使う。
import { Dash } from './cells';

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

export function AccessLogFlags({ flags }: { flags: string[] }) {
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

export function AccessLogSources({ labels }: { labels: string[] }) {
  if (labels.length === 0) return <Dash />;
  return <span title={labels.join(', ')}>{labels.join(', ')}</span>;
}
