// EC2 / ECS のターミナルセッションをターミナルドックに開く関数。
// ターミナル本体は App 直下の常駐ドック (components/Terminal/TerminalDock.tsx) にマウントされるため、
// Drawer 側のランチャー (DrawerTerminal.tsx、Drawer.tsx の ECS Tasks タブ) はここを呼ぶだけにする。
// コンポーネントファイル (DrawerTerminal.tsx) に置くと、コンポーネント以外の export により
// react-refresh/only-export-components の警告が出るため、専用ファイルに分離する。
import type { BaseRow } from '../types/common';
import { ec2SessionUrl, ecsExecUrl } from '../api/terminal';
import { arnSuffix } from './format';
import { openTerminalSession } from '../hooks/useTerminalSessions';

// EC2 インスタンスへのセッションをターミナルドックに開く。
// EC2 は instance id (resource.id) だけで SSM Start Session を開始できる。
export function openEC2TerminalSession(profile: string, region: string, resource: BaseRow): string {
  return openTerminalSession({
    kind: 'ec2',
    profile,
    region,
    label: resource.name || resource.id,
    wsUrl: ec2SessionUrl(profile, resource.id, region),
  });
}

// ECS タスクコンテナへの Exec セッションをターミナルドックに開く。
// task はタスク ARN でも ID でもよい (ARN は末尾のセグメントへ正規化する)。
export function openECSTerminalSession(
  profile: string,
  region: string,
  cluster: string,
  task: string,
  container: string,
): string {
  const taskId = arnSuffix(task);
  return openTerminalSession({
    kind: 'ecs',
    profile,
    region,
    label: `${cluster} / ${taskId} / ${container}`,
    wsUrl: ecsExecUrl(profile, cluster, taskId, container, region),
  });
}
