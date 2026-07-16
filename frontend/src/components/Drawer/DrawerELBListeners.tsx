// ELB の Listener 一覧と、選択した Listener に紐づく Rule 一覧を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useELBListeners, useELBRules } from '../../api/queries';
import { elbListenerColumns, elbRuleColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
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
  const { data, isLoading } = useELBRules(profile, region, listenerArn);
  const rows = useMemo<ELBRuleTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn })),
    [data],
  );

  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <h3>Rules ({rows.length})</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable rows={rows} columns={elbRuleColumns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}

export function DrawerELBListeners({ profile, region, lbArn }: DrawerELBListenersProps) {
  const { data, isLoading } = useELBListeners(profile, region, lbArn);
  const [selectedArn, setSelectedArn] = useState<string | null>(null);
  const rows = useMemo<ELBListenerTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn })),
    [data],
  );

  return (
    <div className="section">
      <h3>Listeners ({rows.length})</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
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
    </div>
  );
}
