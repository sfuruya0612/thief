// ECS クラスタのコンテナインスタンス (ECS on EC2) ごとに稼働タスクを表示する Drawer タブ。
// container-instances と tasks の 2 クエリを合成し、タスク側の containerInstanceArn で
// グルーピングする。Fargate のタスク (containerInstanceArn が空) はどこにも表示しない。
// 表示規則: どちらかが loading なら Loading のみ、どちらかが error なら Error のみ
// (両方 error のときはインスタンス側を優先)。片方のデータだけで見出しやタスクを出すと
// 誤ったグルーピングに見えるため、issues/0075 の「data があれば上部に Error」規則は採らない。
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useECSContainerInstances, useECSTasks } from '../../api/queries';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import { StatusBadge } from '../primitives';
import type { ECSContainerInstanceRow, ECSTaskRow } from '../../types/aws';

export interface DrawerECSContainerInstancesProps {
  profile: string;
  region: string;
  cluster: string;
}

interface Grouped {
  byArn: Map<string, ECSTaskRow[]>;
  unknown: ECSTaskRow[];
}

// タスクを containerInstanceArn でインスタンスに振り分ける。ARN が空 (Fargate) は捨て、
// 一覧に無い ARN は unknown に集める。
function groupTasksByContainerInstance(
  instances: ECSContainerInstanceRow[],
  tasks: ECSTaskRow[],
): Grouped {
  const byArn = new Map<string, ECSTaskRow[]>();
  for (const inst of instances) byArn.set(inst.arn, []);
  const unknown: ECSTaskRow[] = [];
  for (const task of tasks) {
    if (task.containerInstanceArn === '') continue;
    const bucket = byArn.get(task.containerInstanceArn);
    if (bucket) bucket.push(task);
    else unknown.push(task);
  }
  return { byArn, unknown };
}

function resourceText(remaining: number | null, registered: number | null): string {
  const fmt = (v: number | null) => (v === null ? '-' : String(v));
  return `${fmt(remaining)} / ${fmt(registered)}`;
}

const EMPTY_STYLE = { textAlign: 'center', padding: 20, color: 'var(--text-3)' } as const;

function TaskTable({ tasks }: { tasks: ECSTaskRow[] }) {
  const { t } = useTranslation('drawerAws');
  return (
    <table className="dt">
      <colgroup>
        <col style={{ width: '40%' }} />
        <col style={{ width: '20%' }} />
        <col style={{ width: '10%' }} />
        <col style={{ width: '10%' }} />
        <col style={{ width: '20%' }} />
      </colgroup>
      <thead>
        <tr>
          <th>Group</th>
          <th>Last status</th>
          <th>CPU</th>
          <th>Memory</th>
          <th>Started at</th>
        </tr>
      </thead>
      <tbody>
        {tasks.length === 0 ? (
          <tr>
            <td colSpan={5} style={EMPTY_STYLE}>
              {t('ecsContainerInstances.noTasks')}
            </td>
          </tr>
        ) : (
          tasks.map((task) => (
            <tr key={task.arn}>
              <td className="truncate" title={task.arn}>
                {task.group}
              </td>
              <td>
                <StatusBadge state={task.lastStatus} />
              </td>
              <td>{task.cpu || '-'}</td>
              <td>{task.memory || '-'}</td>
              <td>{task.startedAt || '-'}</td>
            </tr>
          ))
        )}
      </tbody>
    </table>
  );
}

function InstanceSection({ inst, tasks }: { inst: ECSContainerInstanceRow; tasks: ECSTaskRow[] }) {
  const { t } = useTranslation('drawerAws');
  return (
    <div className="section" data-testid="ecs-container-instance">
      <h3>
        {inst.ec2InstanceId || inst.arn} <StatusBadge state={inst.status} />
      </h3>
      <div className="kv">
        <div className="k">{t('ecsContainerInstances.agentConnected')}</div>
        <div className="v">
          {inst.agentConnected
            ? t('ecsContainerInstances.connected')
            : t('ecsContainerInstances.disconnected')}
        </div>
        <div className="k">{t('ecsContainerInstances.ecsReported')}</div>
        <div className="v">
          {t('ecsContainerInstances.ecsReportedValue', {
            running: inst.runningTasksCount,
            pending: inst.pendingTasksCount,
          })}
        </div>
        <div className="k">{t('ecsContainerInstances.cpu')}</div>
        <div className="v">{resourceText(inst.remainingCpu, inst.registeredCpu)}</div>
        <div className="k">{t('ecsContainerInstances.memory')}</div>
        <div className="v">{resourceText(inst.remainingMemory, inst.registeredMemory)}</div>
      </div>
      <h4>{t('ecsContainerInstances.tasks', { n: tasks.length })}</h4>
      <TaskTable tasks={tasks} />
    </div>
  );
}

export function DrawerECSContainerInstances({
  profile,
  region,
  cluster,
}: DrawerECSContainerInstancesProps) {
  const { t } = useTranslation('drawerAws');
  const instancesQ = useECSContainerInstances(profile, region, cluster);
  // service 引数を渡さず Tasks タブと同じ queryKey を共有し、tasks の取得を 1 回にする。
  const tasksQ = useECSTasks(profile, region, cluster);

  const instances = instancesQ.data;
  const tasks = tasksQ.data;
  const grouped = useMemo(
    () => groupTasksByContainerInstance(instances ?? [], tasks ?? []),
    [instances, tasks],
  );

  if (instancesQ.isLoading || tasksQ.isLoading) return <DrawerLoading />;
  const error = instancesQ.error ?? tasksQ.error;
  if (error) return <DrawerError error={error} />;
  if (!instances || !tasks) return null;

  return (
    <div>
      <h3>{t('ecsContainerInstances.heading', { n: instances.length })}</h3>
      {instances.length === 0 && <div style={EMPTY_STYLE}>{t('ecsContainerInstances.empty')}</div>}
      {instances.map((inst) => (
        <InstanceSection key={inst.arn} inst={inst} tasks={grouped.byArn.get(inst.arn) ?? []} />
      ))}
      {grouped.unknown.length > 0 && (
        <div className="section" data-testid="ecs-container-instance-unknown">
          <h3>{t('ecsContainerInstances.unknownInstance')}</h3>
          <div style={{ color: 'var(--text-3)', marginBottom: 8 }}>
            {t('ecsContainerInstances.unknownInstanceHint')}
          </div>
          <h4>{t('ecsContainerInstances.tasks', { n: grouped.unknown.length })}</h4>
          <TaskTable tasks={grouped.unknown} />
        </div>
      )}
    </div>
  );
}
