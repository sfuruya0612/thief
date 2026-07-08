// ECS クラスタの Service 一覧を表示する Drawer タブ
import { useMemo } from 'react';
import { useECSServices } from '../../api/queries';
import { ecsServiceColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import type { ECSServiceRow } from '../../types/aws';

export interface DrawerECSServicesProps {
  profile: string;
  region: string;
  cluster: string;
}

type ECSServiceTableRow = ECSServiceRow & { id: string; state: string };

export function DrawerECSServices({ profile, region, cluster }: DrawerECSServicesProps) {
  const { data, isLoading } = useECSServices(profile, region, cluster);
  const rows = useMemo<ECSServiceTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn, state: r.status })),
    [data],
  );

  return (
    <div className="section">
      <h3>Services ({rows.length})</h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <DataTable rows={rows} columns={ecsServiceColumns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}
