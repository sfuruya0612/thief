// RDS インスタンスの DB パラメータグループのパラメータを表示する Drawer の Instance Parameters タブ。
// グループ名は一覧クエリ (useResources) のキャッシュから該当インスタンスを引いて得る。
// 行がまだ無い間はローディング表示を出し、未取得と 0 件を区別する。
import { useMemo, useState } from 'react';
import { useRDSParameters, useResources } from '../../api/queries';
import { rdsFromRaw } from '../../lib/normalize';
import { rdsParameterColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import type { RDSRaw, RDSRow } from '../../types/aws';

export interface DrawerRDSInstanceParametersProps {
  profile: string;
  region: string;
  instance: string;
}

function InstanceParameterTable({
  profile,
  region,
  group,
}: {
  profile: string;
  region: string;
  group: string;
}) {
  const { data, isLoading, error } = useRDSParameters(profile, region, group);
  const rows = useMemo(() => data ?? [], [data]);

  if (isLoading) {
    return (
      <>
        <h3>{group}</h3>
        <DrawerLoading />
      </>
    );
  }
  return (
    <>
      <h3>
        {group}
        {data !== undefined ? ` (${rows.length})` : ''}
      </h3>
      {error != null && <DrawerError error={error} />}
      {data !== undefined && (
        <DataTable
          rows={rows}
          columns={rdsParameterColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </>
  );
}

export function DrawerRDSInstanceParameters({
  profile,
  region,
  instance,
}: DrawerRDSInstanceParametersProps) {
  const { data } = useResources<RDSRaw, RDSRow>('rds', profile, region, rdsFromRaw);
  const row = useMemo(() => data?.find((r) => r.name === instance), [data, instance]);
  const groups = useMemo(() => row?.parameterGroups ?? [], [row]);

  const [selected, setSelected] = useState<string | null>(null);
  // 選択が未確定または一覧に無いグループなら先頭を既定にする (大半のインスタンスは 1 グループ)。
  const active = selected !== null && groups.includes(selected) ? selected : (groups[0] ?? '');

  if (row === undefined) {
    return (
      <div className="section">
        <h3>Instance parameters</h3>
        <DrawerLoading />
      </div>
    );
  }
  if (groups.length === 0) {
    return (
      <div className="section">
        <h3>Instance parameters</h3>
        <p className="muted">No parameter groups.</p>
      </div>
    );
  }
  return (
    <div className="section">
      {groups.length > 1 && (
        <div className="seg" style={{ marginBottom: 12 }}>
          {groups.map((g) => (
            <button key={g} className={active === g ? 'active' : ''} onClick={() => setSelected(g)}>
              {g}
            </button>
          ))}
        </div>
      )}
      <InstanceParameterTable profile={profile} region={region} group={active} />
    </div>
  );
}
