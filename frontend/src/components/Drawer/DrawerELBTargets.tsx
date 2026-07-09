// ELB の Target Group 一覧と、選択した Target Group に紐づくターゲットのヘルス状態を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useELBTargetGroups, useELBTargetHealth } from '../../api/queries';
import { elbTargetGroupColumns, elbTargetHealthColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import type { ELBTargetGroupRow, ELBTargetHealthRow } from '../../types/aws';

export interface DrawerELBTargetsProps {
  profile: string;
  region: string;
  lbArn: string;
}

// DataTable が要求する id/state を arn/targetId から射影した行型
type ELBTargetGroupTableRow = ELBTargetGroupRow & { id: string; state?: string };
type ELBTargetHealthTableRow = ELBTargetHealthRow & { id: string };

function TargetGroupHealth({
  profile,
  region,
  tgArn,
}: {
  profile: string;
  region: string;
  tgArn: string;
}) {
  const { data, isLoading } = useELBTargetHealth(profile, region, tgArn);
  const rows = useMemo<ELBTargetHealthTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: `${r.targetId}:${r.port}` })),
    [data],
  );

  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <h3>Targets ({rows.length})</h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <DataTable
          rows={rows}
          columns={elbTargetHealthColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </div>
  );
}

export function DrawerELBTargets({ profile, region, lbArn }: DrawerELBTargetsProps) {
  const { data, isLoading } = useELBTargetGroups(profile, region, lbArn);
  const [selectedArn, setSelectedArn] = useState<string | null>(null);
  const rows = useMemo<ELBTargetGroupTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.arn })),
    [data],
  );

  return (
    <div className="section">
      <h3>Target groups ({rows.length})</h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <>
          <DataTable
            rows={rows}
            columns={elbTargetGroupColumns}
            onSelect={(r) => setSelectedArn(r.arn)}
            selectedId={selectedArn}
          />
          {selectedArn && (
            <TargetGroupHealth profile={profile} region={region} tgArn={selectedArn} />
          )}
        </>
      )}
    </div>
  );
}
