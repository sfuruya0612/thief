// 所属 DB クラスターの DB クラスターパラメータグループのパラメータを表示する
// Drawer の Cluster Parameters タブ。clusterId は一覧クエリ (useResources) のキャッシュから
// 該当インスタンスを引いて得る。クラスターに属さないインスタンスは空表示を出す。
// 見出しは Instance Parameters タブと揃えてクラスターパラメータグループ名を表示する。
// グループ名が取得できるまで (ローディング中や group_name 空) は clusterId にフォールバックする。
import { useMemo } from 'react';
import { useRDSClusterParameters, useResources } from '../../api/queries';
import { rdsFromRaw } from '../../lib/normalize';
import { rdsParameterColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import type { RDSRaw, RDSRow } from '../../types/aws';

export interface DrawerRDSClusterParametersProps {
  profile: string;
  region: string;
  instance: string;
}

export function DrawerRDSClusterParameters({
  profile,
  region,
  instance,
}: DrawerRDSClusterParametersProps) {
  const { data } = useResources<RDSRaw, RDSRow>('rds', profile, region, rdsFromRaw);
  const row = useMemo(() => data?.find((r) => r.name === instance), [data, instance]);
  const clusterId = row?.clusterId ?? '';
  // clusterId が空の間は enabled 制御により取得が発火しない。
  const query = useRDSClusterParameters(profile, region, clusterId);
  const rows = useMemo(() => query.data?.parameters ?? [], [query.data]);
  const heading = query.data?.groupName || clusterId;

  if (row === undefined) {
    return (
      <div className="section">
        <h3>Cluster parameters</h3>
        <DrawerLoading />
      </div>
    );
  }
  if (!clusterId) {
    return (
      <div className="section">
        <h3>Cluster parameters</h3>
        <p className="muted">Not part of a DB cluster.</p>
      </div>
    );
  }
  if (query.isLoading) {
    return (
      <div className="section">
        <h3>{clusterId}</h3>
        <DrawerLoading />
      </div>
    );
  }
  return (
    <div className="section">
      <h3>
        {heading}
        {query.data !== undefined ? ` (${rows.length})` : ''}
      </h3>
      {query.error != null && <DrawerError error={query.error} />}
      {query.data !== undefined && (
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
