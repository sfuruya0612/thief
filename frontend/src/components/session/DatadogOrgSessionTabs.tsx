// Datadog 用セッションタブの組立レイヤ (GcpSessionTabs と同じ構成)。
// 組織ごとに OAuth トークンが別なので、未ログインの組織のタブを開いたり選んだりした
// 時点でログインを自動的に開始する。明示的なログインボタンは置かない (AWS / GCP の
// 「開けば繋がる」操作感に合わせる)。ログインは click ハンドラの同期処理で認可タブを
// 開く必要があるため、begin の呼び出しは各ハンドラから直接行う。
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useRefreshDatadogOrgs } from '../../api/queries';
import type { DatadogOrgSessions } from '../../hooks/useDatadogOrgs';
import { useDatadogLoginFlow } from '../../hooks/useDatadogLogin';
import { datadogOrgPickerItems, resolveDatadogRequestOrg } from '../../lib/sessionMeta';
import { Icons } from '../icons/Icons';
import { AddSessionPicker } from './AddSessionPicker';
import { SessionTabs, type SessionTabItem } from './SessionTabs';

export interface DatadogOrgSessionTabsProps {
  sessions: DatadogOrgSessions;
}

export function DatadogOrgSessionTabs({ sessions }: DatadogOrgSessionTabsProps) {
  const { t } = useTranslation('session');
  const { orgs, openOrgs, activeOrg, isError } = sessions;
  const refreshOrgs = useRefreshDatadogOrgs();
  const flow = useDatadogLoginFlow();

  const items = useMemo<SessionTabItem[]>(() => {
    const names = new Map(orgs.map((o) => [o.id, o.name]));
    // 一覧に無い組織 (取得前や一覧から消えた組織) は id をそのままラベルにする。
    return openOrgs.map((id) => ({ id, label: names.get(id) ?? id, env: 'default' }));
  }, [orgs, openOrgs]);

  const missingIds = useMemo(() => {
    if (orgs.length === 0) return [];
    const known = new Set(orgs.map((o) => o.id));
    return openOrgs.filter((id) => !known.has(id));
  }, [orgs, openOrgs]);

  const pickerItems = useMemo(() => datadogOrgPickerItems(orgs, openOrgs), [orgs, openOrgs]);

  // 未ログインの組織を選んだときだけログインを始める。一覧に無い組織はログイン状態が
  // 分からないため始めない (別の組織の認可を勝手に開始しない)。
  // 親組織自身のタブ (isSelf) は、Datadog 側の id ではなく org: '' でログインを開始する。
  // id をそのまま渡すと親組織向けの Sub Organization ログインが新たに始まってしまい、
  // 既存の親組織トークン (org == '') と結び付かないまま「ログインしても未ログイン表示の
  // まま」になる (issue 0171)。isSelf からの org 解決は resolveDatadogRequestOrg に
  // 集約する (Cost/Dashboards/Metrics の取得先解決と同じロジック)。
  const loginIfNeeded = (id: string) => {
    const org = orgs.find((o) => o.id === id);
    if (!org || org.loggedIn || flow.loggingIn) return;
    flow.begin(resolveDatadogRequestOrg(orgs, id));
  };

  return (
    <SessionTabs
      items={items}
      activeId={activeOrg}
      addLabel={t('datadogOrgSessionTabs.addLabel')}
      missingIds={missingIds}
      picker={(close, visibleCount) => (
        <AddSessionPicker
          items={pickerItems}
          placeholder={t('datadogOrgSessionTabs.searchPlaceholder')}
          headerNote="GET /api/v2/org"
          headerAction={
            <button
              className="btn sm ghost"
              style={{ padding: '1px 4px' }}
              title="Refresh organization list from Datadog"
              disabled={refreshOrgs.isPending}
              onClick={() => refreshOrgs.mutate()}
            >
              <Icons.refresh size={11} />
            </button>
          }
          footerHint={t('datadogOrgSessionTabs.footerHint')}
          emptyText={t('datadogOrgSessionTabs.emptyText')}
          loadError={isError}
          onRetry={() => refreshOrgs.mutate()}
          narrow
          onSelect={(id) => {
            loginIfNeeded(id);
            sessions.openOrg(id);
            sessions.swapOrgToVisible(id, visibleCount);
            close();
          }}
          onClose={close}
        />
      )}
      onActivate={(id) => {
        loginIfNeeded(id);
        sessions.activateOrg(id);
      }}
      onClose={sessions.closeOrg}
      onReorder={sessions.moveOrg}
      onSwapToVisible={(id, visibleCount) => {
        loginIfNeeded(id);
        sessions.swapOrgToVisible(id, visibleCount);
      }}
    />
  );
}
