// ECS クラスタの Task 一覧を表示する Drawer タブ
import { useMemo } from 'react';
import { useECSTasks } from '../../api/queries';
import { ecsTaskColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import type { ECSTaskRow } from '../../types/aws';

export interface DrawerECSTasksProps {
  profile: string;
  region: string;
  cluster: string;
}

type ECSTaskTableRow = ECSTaskRow & { id: string; state: string };

export function DrawerECSTasks({ profile, region, cluster }: DrawerECSTasksProps) {
  const { data, isLoading } = useECSTasks(profile, region, cluster);
  const rows = useMemo<ECSTaskTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn, state: r.lastStatus })),
    [data],
  );

  return (
    <div className="section">
      <h3>Tasks ({rows.length})</h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <DataTable rows={rows} columns={ecsTaskColumns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}
