// ElastiCache クラスタに紐づく Cache パラメータグループのパラメータ一覧を表示する Drawer タブ。
// クラスタは単一のパラメータグループを持つため、グループ選択は挟まず直接表示する。
// パラメータグループ名は一覧クエリ (useResources) のキャッシュから該当クラスタを引いて得る。
import { useMemo } from 'react';
import { useCacheParameters, useResources } from '../../api/queries';
import { cacheFromRaw } from '../../lib/normalize';
import { cacheParameterColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import type { CacheRaw, CacheRow } from '../../types/aws';

export interface DrawerCacheParametersProps {
  profile: string;
  region: string;
  cluster: string;
}

export function DrawerCacheParameters({ profile, region, cluster }: DrawerCacheParametersProps) {
  const { data } = useResources<CacheRaw, CacheRow>('cache', profile, region, cacheFromRaw);
  const group = useMemo(
    () => data?.find((r) => r.name === cluster)?.parameterGroup ?? '',
    [data, cluster],
  );
  const { data: params, isLoading } = useCacheParameters(profile, region, group);
  const rows = useMemo(() => params ?? [], [params]);

  return (
    <div className="section">
      <h3>
        {group || 'Parameters'} ({rows.length})
      </h3>
      {!group ? (
        <p className="muted">No parameter group.</p>
      ) : isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable
          rows={rows}
          columns={cacheParameterColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </div>
  );
}
