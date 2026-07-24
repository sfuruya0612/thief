// RDS インスタンスに紐づく DB パラメータグループ (instance) と、所属 DB クラスターの
// DB クラスターパラメータグループ (cluster) を種別付きセグメントで切り替えて表示する Drawer タブ。
// パラメータグループ名と clusterId は一覧クエリ (useResources) のキャッシュから該当インスタンスを引いて得る。
import { useMemo, useState } from 'react';
import { useRDSClusterParameters, useRDSParameters, useResources } from '../../api/queries';
import { rdsFromRaw } from '../../lib/normalize';
import { rdsParameterColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import type { RDSRaw, RDSRow } from '../../types/aws';

export interface DrawerRDSParametersProps {
  profile: string;
  region: string;
  instance: string;
}

type ParameterSegment = { kind: 'instance'; name: string } | { kind: 'cluster'; name: string };

function segmentKey(s: ParameterSegment): string {
  return `${s.kind}:${s.name}`;
}

function segmentLabel(s: ParameterSegment): string {
  return s.kind === 'cluster' ? `Cluster: ${s.name}` : s.name;
}

function RDSParameterTable({
  profile,
  region,
  segment,
}: {
  profile: string;
  region: string;
  segment: ParameterSegment;
}) {
  const instanceQuery = useRDSParameters(
    profile,
    region,
    segment.kind === 'instance' ? segment.name : '',
  );
  const clusterQuery = useRDSClusterParameters(
    profile,
    region,
    segment.kind === 'cluster' ? segment.name : '',
  );
  const { data, isLoading } = segment.kind === 'instance' ? instanceQuery : clusterQuery;
  const rows = useMemo(() => data ?? [], [data]);

  return (
    <div
      className="section"
      style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 16 }}
    >
      <h3>
        {segmentLabel(segment)} ({rows.length})
      </h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable
          rows={rows}
          columns={rdsParameterColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </div>
  );
}

export function DrawerRDSParameters({ profile, region, instance }: DrawerRDSParametersProps) {
  const { data } = useResources<RDSRaw, RDSRow>('rds', profile, region, rdsFromRaw);
  const row = useMemo(() => data?.find((r) => r.name === instance), [data, instance]);
  const groups = row?.parameterGroups ?? [];
  const clusterId = row?.clusterId ?? '';

  const segments = useMemo<ParameterSegment[]>(() => {
    const instanceSegments: ParameterSegment[] = groups.map((g) => ({
      kind: 'instance' as const,
      name: g,
    }));
    return clusterId
      ? [...instanceSegments, { kind: 'cluster' as const, name: clusterId }]
      : instanceSegments;
  }, [groups, clusterId]);

  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  // 選択が未確定なら先頭セグメントを既定にする (大半のインスタンスはセグメントが 1 つ)。
  const active = segments.find((s) => segmentKey(s) === selectedKey) ?? segments[0] ?? null;

  return (
    <div className="section">
      <h3>Parameter groups ({segments.length})</h3>
      {segments.length === 0 ? (
        <p className="muted">No parameter groups.</p>
      ) : (
        <>
          {segments.length > 1 && (
            <div className="seg" style={{ marginBottom: 12 }}>
              {segments.map((s) => (
                <button
                  key={segmentKey(s)}
                  className={active && segmentKey(active) === segmentKey(s) ? 'active' : ''}
                  onClick={() => setSelectedKey(segmentKey(s))}
                >
                  {segmentLabel(s)}
                </button>
              ))}
            </div>
          )}
          {active && <RDSParameterTable profile={profile} region={region} segment={active} />}
        </>
      )}
    </div>
  );
}
