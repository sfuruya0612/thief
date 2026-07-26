// ELB の Listener 一覧と、選択した Listener に紐づく Rule 一覧を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useELBListeners, useELBRules } from '../../api/queries';
import { elbListenerColumns, elbRuleColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import type { ELBListenerRow, ELBRuleRow } from '../../types/aws';

export interface DrawerELBListenersProps {
  profile: string;
  region: string;
  lbArn: string;
}

// DataTable が要求する id/state を arn から射影した行型
type ELBListenerTableRow = ELBListenerRow & { id: string; state?: string };
type ELBRuleTableRow = ELBRuleRow & { id: string; state?: string };

function ListenerRules({
  profile,
  region,
  listenerArn,
}: {
  profile: string;
  region: string;
  listenerArn: string;
}) {
  const { data, isLoading, error } = useELBRules(profile, region, listenerArn);
  const rows = useMemo<ELBRuleTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn })),
    [data],
  );

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <h3>Rules{data !== undefined ? ` (${rows.length})` : ''}</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <>
          {error != null && <DrawerError error={error} />}
          {data !== undefined && (
            <DataTable rows={rows} columns={elbRuleColumns} onSelect={() => {}} selectedId={null} />
          )}
        </>
      )}
    </div>
  );
}

export function DrawerELBListeners({ profile, region, lbArn }: DrawerELBListenersProps) {
  const { data, isLoading, error } = useELBListeners(profile, region, lbArn);
  const [selectedArn, setSelectedArn] = useState<string | null>(null);
  const rows = useMemo<ELBListenerTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn })),
    [data],
  );

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <div className="section">
      <h3>Listeners{data !== undefined ? ` (${rows.length})` : ''}</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <>
          {error != null && <DrawerError error={error} />}
          {data !== undefined && (
            <>
              <DataTable
                rows={rows}
                columns={elbListenerColumns}
                onSelect={(r) => setSelectedArn(r.arn)}
                selectedId={selectedArn}
              />
              {selectedArn && (
                <ListenerRules profile={profile} region={region} listenerArn={selectedArn} />
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
