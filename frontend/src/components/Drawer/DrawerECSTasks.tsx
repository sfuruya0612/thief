// ECS クラスタの Task 一覧を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useECSTasks } from '../../api/queries';
import { ecsTaskColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { FacetBar } from '../FacetBar';
import type { Filters } from '../FacetBar';
import { StatusBadge } from '../primitives';
import { Icons } from '../icons/Icons';
import type { ECSTaskRow } from '../../types/aws';

export interface DrawerECSTasksProps {
  profile: string;
  region: string;
  cluster: string;
}

// DataTable が要求する id/state を arn/lastStatus から射影した行型
type ECSTaskTableRow = ECSTaskRow & { id: string; state: string };

// タスク選択時に Tasks タブ内へ表示する詳細ペイン
function ECSTaskDetail({ task, onClose }: { task: ECSTaskTableRow; onClose: () => void }) {
  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}>Task detail</h3>
        <button className="x" style={{ marginLeft: 'auto' }} onClick={onClose} title="Close">
          <Icons.x />
        </button>
      </div>
      <div className="kv">
        <div className="k">ARN</div>
        <div className="v mono">{task.arn}</div>
        <div className="k">Group</div>
        <div className="v">{task.group}</div>
        <div className="k">Last status</div>
        <div className="v">
          <StatusBadge state={task.lastStatus} />
        </div>
        <div className="k">Desired status</div>
        <div className="v">{task.desiredStatus}</div>
        <div className="k">Launch type</div>
        <div className="v">{task.launchType}</div>
        <div className="k">CPU</div>
        <div className="v">{task.cpu || '-'}</div>
        <div className="k">Memory</div>
        <div className="v">{task.memory || '-'}</div>
        <div className="k">Started at</div>
        <div className="v">{task.startedAt || '-'}</div>
        <div className="k">Stopped at</div>
        <div className="v">{task.stoppedAt || '-'}</div>
        <div className="k">Stopped reason</div>
        <div className="v">{task.stoppedReason || '-'}</div>
      </div>

      <h3>Containers ({task.containers.length})</h3>
      <table className="dt">
        <colgroup>
          <col style={{ width: '22%' }} />
          <col style={{ width: '30%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '14%' }} />
          <col style={{ width: '8%' }} />
          <col style={{ width: '12%' }} />
        </colgroup>
        <thead>
          <tr>
            <th>Name</th>
            <th>Image</th>
            <th>Last status</th>
            <th>Health</th>
            <th>Exit code</th>
            <th>Reason</th>
          </tr>
        </thead>
        <tbody>
          {task.containers.map((c) => (
            <tr key={c.name}>
              <td className="primary">{c.name}</td>
              <td className="truncate">{c.image || '-'}</td>
              <td>
                <StatusBadge state={c.lastStatus} />
              </td>
              <td>{c.healthStatus || '-'}</td>
              <td>{c.exitCode ?? '-'}</td>
              <td className="truncate">{c.reason || '-'}</td>
            </tr>
          ))}
          {task.containers.length === 0 && (
            <tr>
              <td colSpan={6} style={{ textAlign: 'center', padding: 20, color: 'var(--text-3)' }}>
                No containers
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export function DrawerECSTasks({ profile, region, cluster }: DrawerECSTasksProps) {
  const { data, isLoading } = useECSTasks(profile, region, cluster);
  const [filters, setFilters] = useState<Filters>({});
  const [selectedArn, setSelectedArn] = useState<string | null>(null);
  const rows = useMemo<ECSTaskTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn, state: r.lastStatus })),
    [data],
  );

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (filters.state?.length && !filters.state.includes(r.state)) return false;
      return true;
    });
  }, [rows, filters]);

  const selectedTask = useMemo(
    () => rows.find((r) => r.arn === selectedArn) ?? null,
    [rows, selectedArn],
  );

  return (
    <div className="section">
      <h3>Tasks ({filtered.length})</h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <>
          <FacetBar rows={rows} filters={filters} setFilters={setFilters} />
          <DataTable
            rows={filtered}
            columns={ecsTaskColumns}
            onSelect={(r) => setSelectedArn(r.arn)}
            selectedId={selectedArn}
          />
          {selectedTask && (
            <ECSTaskDetail task={selectedTask} onClose={() => setSelectedArn(null)} />
          )}
        </>
      )}
    </div>
  );
}
