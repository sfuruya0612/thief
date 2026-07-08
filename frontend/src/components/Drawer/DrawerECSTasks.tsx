// ECS クラスタの Task 一覧を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useECSTasks } from '../../api/queries';
import { ecsTaskColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { FacetBar } from '../FacetBar';
import type { Filters } from '../FacetBar';
import type { ECSTaskRow } from '../../types/aws';

export interface DrawerECSTasksProps {
  profile: string;
  region: string;
  cluster: string;
}

// DataTable が要求する id/state を arn/lastStatus から射影した行型
type ECSTaskTableRow = ECSTaskRow & { id: string; state: string };

export function DrawerECSTasks({ profile, region, cluster }: DrawerECSTasksProps) {
  const { data, isLoading } = useECSTasks(profile, region, cluster);
  const [filters, setFilters] = useState<Filters>({});
  const [search, setSearch] = useState('');
  const rows = useMemo<ECSTaskTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn, state: r.lastStatus })),
    [data],
  );

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (search) {
        const q = search.toLowerCase();
        const hay = `${r.group} ${r.arn} ${r.containerNames.join(' ')}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      if (filters.state?.length && !filters.state.includes(r.state)) return false;
      return true;
    });
  }, [rows, filters, search]);

  return (
    <div className="section">
      <h3>Tasks ({filtered.length})</h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <>
          <FacetBar
            rows={rows}
            filters={filters}
            setFilters={setFilters}
            search={search}
            setSearch={setSearch}
          />
          <DataTable
            rows={filtered}
            columns={ecsTaskColumns}
            onSelect={() => {}}
            selectedId={null}
          />
        </>
      )}
    </div>
  );
}
