// CloudFormation Overview タブの追加情報 (Description / Parameters / Outputs)。
// 一覧 API だけでは取得できないスタック詳細を別 API から補完する。
import { useCFNStackDetail } from '../../api/queries';
import { DrawerLoading } from './DrawerLoading';

export interface DrawerCFNOverviewExtraProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNOverviewExtra({ profile, region, stack }: DrawerCFNOverviewExtraProps) {
  const { data, isLoading } = useCFNStackDetail(profile, region, stack);

  if (isLoading) return <DrawerLoading />;
  if (!data) return null;

  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <h3>Description</h3>
      <p style={{ color: 'var(--text-2)' }}>{data.description || '—'}</p>

      <h3>Parameters ({data.parameters.length})</h3>
      <div className="kv">
        {data.parameters.map((p) => (
          <div key={p.key} style={{ display: 'contents' }}>
            <div className="k">{p.key}</div>
            <div className="v">{p.resolvedValue || p.value || '—'}</div>
          </div>
        ))}
      </div>

      <h3>Outputs ({data.outputs.length})</h3>
      <div className="kv">
        {data.outputs.map((o) => (
          <div key={o.key} style={{ display: 'contents' }}>
            <div className="k">{o.key}</div>
            <div className="v">{o.value || '—'}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
