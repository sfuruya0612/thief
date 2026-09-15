// Drawer の Terminal タブ本体。
// ターミナル本体は App 直下の常駐ドック (components/Terminal/TerminalDock.tsx) にマウントされるため、
// このタブはドックへセッションを開くランチャーだけを担う (同じセッションのマウント位置が
// 2 つにならないよう、ここでは Terminal を描画しない)。
// EC2 は instance id (resource.id) だけで SSM Start Session を開始できる。
// ECS は cluster (resource.name; ARN ではなく bare name。パスセグメントとして "/" を含む ARN は使えない) から
// タスク一覧・コンテナ一覧を取得し、選択した上で Exec Command を開始する
// (タスクが単一/コンテナが単一の場合は自動選択する)。
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { BaseRow } from '../../types/common';
import { useECSContainers, useECSTasks } from '../../api/queries';
import { arnSuffix } from '../../lib/format';
import { openEC2TerminalSession, openECSTerminalSession } from '../../lib/terminalLaunchers';
import { Icons } from '../icons/Icons';

export interface DrawerTerminalProps {
  service: string;
  profile: string;
  region: string;
  resource: BaseRow;
}

export function DrawerTerminal({ service, profile, region, resource }: DrawerTerminalProps) {
  if (service === 'ec2') {
    return <EC2ExecLauncher profile={profile} region={region} resource={resource} />;
  }
  if (service === 'ecs') {
    return <ECSExecLauncher profile={profile} region={region} cluster={resource.name} />;
  }
  return null;
}

function ConnectButton({ disabled, onClick }: { disabled?: boolean; onClick: () => void }) {
  const { t } = useTranslation('drawerStorage');
  return (
    <button className="btn sm" disabled={disabled} onClick={onClick}>
      <Icons.terminal size={12} /> {t('terminal.connect')}
    </button>
  );
}

function EC2ExecLauncher({
  profile,
  region,
  resource,
}: {
  profile: string;
  region: string;
  resource: BaseRow;
}) {
  const { t } = useTranslation('drawerStorage');
  return (
    <div className="col" style={{ gap: 10 }}>
      <div className="row" style={{ gap: 8 }}>
        <ConnectButton onClick={() => openEC2TerminalSession(profile, region, resource)} />
      </div>
      <div className="empty-hint">{t('terminal.launcherHint')}</div>
    </div>
  );
}

function ECSExecLauncher({
  profile,
  region,
  cluster,
}: {
  profile: string;
  region: string;
  cluster: string;
}) {
  const { t } = useTranslation('drawerStorage');
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
    return <div className="empty-hint">{t('drawerTerminal.loadingTasks')}</div>;
  }
  if (!tasks || tasks.length === 0) {
    return <div className="empty-hint">{t('drawerTerminal.noRunningTasks')}</div>;
  }

  const execEnabledContainers = containers?.filter((c) => c.execEnabled) ?? [];

  return (
    <div className="col" style={{ gap: 10 }}>
      <div className="row" style={{ gap: 8 }}>
        {tasks.length > 1 && (
          <select
            className="btn sm"
            value={taskArn}
            onChange={(e) => setTaskArn(e.target.value)}
            title="Task"
          >
            <option value="">{t('drawerTerminal.selectTask')}</option>
            {tasks.map((t) => (
              <option key={t.arn} value={t.arn}>
                {t.group || '-'} / {arnSuffix(t.arn)}
                {t.startedAt ? ` (${t.startedAt})` : ''}
              </option>
            ))}
          </select>
        )}
        {taskArn && containersLoading && (
          <span className="muted">{t('drawerTerminal.loadingContainers')}</span>
        )}
        {taskArn && !containersLoading && execEnabledContainers.length > 1 && (
          <select
            className="btn sm"
            value={containerName}
            onChange={(e) => setContainerName(e.target.value)}
            title="Container"
          >
            <option value="">{t('drawerTerminal.selectContainer')}</option>
            {execEnabledContainers.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
        )}
        {/* 選択を変えるたびにセッションが開かないよう、接続は明示的なボタン操作に限る */}
        <ConnectButton
          disabled={!taskArn || !containerName}
          onClick={() => openECSTerminalSession(profile, region, cluster, taskArn, containerName)}
        />
      </div>

      {taskArn && !containersLoading && execEnabledContainers.length === 0 && (
        <div className="empty-hint">{t('drawerTerminal.noExecContainers')}</div>
      )}

      <div className="empty-hint">{t('terminal.launcherHint')}</div>
    </div>
  );
}
