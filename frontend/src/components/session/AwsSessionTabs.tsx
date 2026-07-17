// AWS 用セッションタブの組立レイヤ。useProfiles (App.tsx で 1 回だけ呼ぶ) の
// 状態を SessionTabs / AddSessionPicker の表示用データに還元して配線する。
import { useMemo } from 'react';
import type { AwsSessions } from '../../hooks/useProfiles';
import { awsPickerItems } from '../../lib/sessionMeta';
import { AddSessionPicker } from './AddSessionPicker';
import { SessionTabs, type SessionTabItem } from './SessionTabs';

export interface AwsSessionTabsProps {
  sessions: AwsSessions;
}

export function AwsSessionTabs({ sessions }: AwsSessionTabsProps) {
  const { profiles, openProfiles, activeProfile } = sessions;

  // AWS のドットは環境色を使わない (モック 4b: 環境による特別扱いなし)。
  const items = useMemo<SessionTabItem[]>(
    () => openProfiles.map((name) => ({ id: name, label: name, env: 'default' })),
    [openProfiles],
  );

  // 一覧が取得できているときだけ missing 判定する (API エラー時に全タブが
  // グレーになるのを防ぐ)。
  const missingIds = useMemo(() => {
    if (profiles.length === 0) return [];
    const known = new Set(profiles.map((p) => p.name));
    return openProfiles.filter((name) => !known.has(name));
  }, [profiles, openProfiles]);

  const pickerItems = useMemo(
    () => awsPickerItems(profiles, openProfiles),
    [profiles, openProfiles],
  );

  return (
    <SessionTabs
      items={items}
      activeId={activeProfile}
      addLabel="＋ プロファイルを追加"
      missingIds={missingIds}
      picker={(close, visibleCount) => (
        <AddSessionPicker
          items={pickerItems}
          placeholder="プロファイルを検索…"
          headerNote={`~/.aws/config · ${profiles.length}件`}
          footerHint="↑↓ で選択 · ⏎ で開く · 期限切れは開いた後に再認証を案内"
          emptyText="一致するプロファイルがありません"
          onSelect={(id) => {
            sessions.openProfile(id);
            // オーバーフロー中でも追加したタブが表示域に入るよう右端と入替える
            sessions.swapProfileToVisible(id, visibleCount);
            close();
          }}
          onClose={close}
        />
      )}
      onActivate={sessions.activateProfile}
      onClose={sessions.closeProfile}
      onReorder={sessions.moveProfile}
      onSwapToVisible={sessions.swapProfileToVisible}
    />
  );
}
