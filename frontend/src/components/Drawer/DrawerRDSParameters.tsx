// RDS インスタンスに紐づく DB パラメータグループを選び、そのパラメータ一覧を表示する Drawer タブ。
// パラメータグループ名は一覧クエリ (useResources) のキャッシュから該当インスタンスを引いて得る。
import { useMemo, useState } from 'react';
import { useRDSParameters, useResources } from '../../api/queries';
import { rdsFromRaw } from '../../lib/normalize';
import { rdsParameterColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import type { RDSRaw, RDSRow } from '../../types/aws';

export interface DrawerRDSParametersProps {
  profile: string;
  region: string;
  instance: string;
}

function RDSParameterTable({
  profile,
  region,
  group,
}: {
  profile: string;
  region: string;
  group: string;
}) {
  const { data, isLoading } = useRDSParameters(profile, region, group);
  const rows = useMemo(() => data ?? [], [data]);

  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <h3>
        {group} ({rows.length})
      </h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable
          rows={rows}
          columns={rdsParameterColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </div>
  );
}

export function DrawerRDSParameters({ profile, region, instance }: DrawerRDSParametersProps) {
  const { data } = useResources<RDSRaw, RDSRow>('rds', profile, region, rdsFromRaw);
  const groups = useMemo(
    () => data?.find((r) => r.name === instance)?.parameterGroups ?? [],
    [data, instance],
  );
  const [selected, setSelected] = useState<string | null>(null);
  // 選択が未確定なら先頭グループを既定にする (大半のインスタンスはグループが 1 つ)。
  const active = selected ?? groups[0] ?? null;

  return (
    <div className="section">
      <h3>Parameter groups ({groups.length})</h3>
      {groups.length === 0 ? (
        <p className="muted">No parameter groups.</p>
      ) : (
        <>
          {groups.length > 1 && (
            <div className="seg" style={{ marginBottom: 12 }}>
              {groups.map((g) => (
                <button
                  key={g}
                  className={active === g ? 'active' : ''}
                  onClick={() => setSelected(g)}
                >
                  {g}
                </button>
              ))}
            </div>
          )}
          {active && <RDSParameterTable profile={profile} region={region} group={active} />}
        </>
      )}
    </div>
  );
}
