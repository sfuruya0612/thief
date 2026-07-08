// Drawer の Terminal タブ本体。
// EC2 は instance id (resource.id) だけで SSM Start Session を開始できる。
// ECS は cluster (resource.name; ARN ではなく bare name。パスセグメントとして "/" を含む ARN は使えない) から
// タスク一覧・コンテナ一覧を取得し、選択した上で Exec Command を開始する
// (タスクが単一/コンテナが単一の場合は自動選択する)。
import { useEffect, useState } from 'react';
import type { BaseRow } from '../../types/common';
import { ec2SessionUrl, ecsExecUrl } from '../../api/terminal';
import { useECSContainers, useECSTasks } from '../../api/queries';
import { arnSuffix } from '../../lib/format';
import { Terminal } from '../Terminal/Terminal';

export interface DrawerTerminalProps {
  service: string;
  profile: string;
  region: string;
  resource: BaseRow;
}

export function DrawerTerminal({ service, profile, region, resource }: DrawerTerminalProps) {
  if (service === 'ec2') {
    return <Terminal wsUrl={ec2SessionUrl(profile, resource.id, region)} />;
  }
  if (service === 'ecs') {
    return <ECSExecTerminal profile={profile} region={region} cluster={resource.name} />;
  }
  return null;
}

function ECSExecTerminal({
  profile,
  region,
  cluster,
}: {
  profile: string;
  region: string;
  cluster: string;
}) {
  const { data: tasks, isLoading: tasksLoading } = useECSTasks(profile, region, cluster);
  const [taskArn, setTaskArn] = useState('');

  useEffect(() => {
    // タスクが単一ならそのまま自動選択、複数ならユーザー選択を待つ
    if (tasks && tasks.length === 1) {
      setTaskArn(tasks[0].arn);
    } else {
      setTaskArn('');
    }
  }, [tasks]);

  const task = arnSuffix(taskArn);
  const { data: containers, isLoading: containersLoading } = useECSContainers(
    profile,
    region,
    cluster,
    task,
  );
  const [containerName, setContainerName] = useState('');

  useEffect(() => {
    const execEnabled = containers?.filter((c) => c.execEnabled) ?? [];
    if (execEnabled.length === 1) {
      setContainerName(execEnabled[0].name);
    } else {
      setContainerName('');
    }
  }, [containers]);

  if (tasksLoading) {
    return <div className="empty-hint">タスク一覧を取得中...</div>;
  }
  if (!tasks || tasks.length === 0) {
    return <div className="empty-hint">実行中のタスクがありません</div>;
  }

  const execEnabledContainers = containers?.filter((c) => c.execEnabled) ?? [];

  return (
    <div className="col" style={{ height: '100%', gap: 10 }}>
      <div className="row" style={{ gap: 8 }}>
        {tasks.length > 1 && (
          <select
            className="btn sm"
            value={taskArn}
            onChange={(e) => setTaskArn(e.target.value)}
            title="Task"
          >
            <option value="">タスクを選択</option>
            {tasks.map((t) => (
              <option key={t.arn} value={t.arn}>
                {arnSuffix(t.arn)}
              </option>
            ))}
          </select>
        )}
        {taskArn && containersLoading && <span className="muted">コンテナ一覧を取得中...</span>}
        {taskArn && !containersLoading && execEnabledContainers.length > 1 && (
          <select
            className="btn sm"
            value={containerName}
            onChange={(e) => setContainerName(e.target.value)}
            title="Container"
          >
            <option value="">コンテナを選択</option>
            {execEnabledContainers.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
        )}
      </div>

      {taskArn && !containersLoading && execEnabledContainers.length === 0 && (
        <div className="empty-hint">
          Exec 可能なコンテナがありません (enableExecuteCommand を確認)
        </div>
      )}

      {taskArn && containerName && (
        <Terminal wsUrl={ecsExecUrl(profile, cluster, task, containerName, region)} />
      )}
    </div>
  );
}
