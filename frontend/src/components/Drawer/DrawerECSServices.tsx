// ECS クラスタの Service 一覧を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useECSServices } from '../../api/queries';
import { ecsServiceColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import { FacetBar } from '../FacetBar';
import type { Filters } from '../FacetBar';
import type { ECSServiceRow } from '../../types/aws';

export interface DrawerECSServicesProps {
  profile: string;
  region: string;
  cluster: string;
}

// DataTable が要求する id/state を arn/status から射影した行型
type ECSServiceTableRow = ECSServiceRow & { id: string; state: string };

export function DrawerECSServices({ profile, region, cluster }: DrawerECSServicesProps) {
  const { data, isLoading, error } = useECSServices(profile, region, cluster);
  const [filters, setFilters] = useState<Filters>({});
  const rows = useMemo<ECSServiceTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn, state: r.status })),
    [data],
  );

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (filters.state?.length && !filters.state.includes(r.state)) return false;
      return true;
    });
  }, [rows, filters]);

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <div className="section">
      <h3>Services{data !== undefined ? ` (${filtered.length})` : ''}</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <>
          {error != null && <DrawerError error={error} />}
          {data !== undefined && (
            <>
              <FacetBar rows={rows} filters={filters} setFilters={setFilters} />
              <DataTable
                rows={filtered}
                columns={ecsServiceColumns}
                onSelect={() => {}}
                selectedId={null}
              />
            </>
          )}
        </>
      )}
    </div>
  );
}
