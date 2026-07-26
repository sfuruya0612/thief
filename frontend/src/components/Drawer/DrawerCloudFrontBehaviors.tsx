// CloudFront ディストリビューションのキャッシュビヘイビア一覧を表示する Drawer タブ。
// ビヘイビアは backend で該当ディストリビューションに埋め込まれて返るため、専用エンドポイントは無く、
// 一覧クエリ (useResources) のキャッシュから id で該当行を引き直して behaviors を得る。
import { useMemo } from 'react';
import { useResources } from '../../api/queries';
import { cloudfrontFromRaw } from '../../lib/normalize';
import { cloudfrontBehaviorColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import type { CloudFrontRaw, CloudFrontRow } from '../../types/aws';

export interface DrawerCloudFrontBehaviorsProps {
  profile: string;
  region: string;
  id: string;
}

export function DrawerCloudFrontBehaviors({ profile, region, id }: DrawerCloudFrontBehaviorsProps) {
  const { data, isLoading: listLoading } = useResources<CloudFrontRaw, CloudFrontRow>(
    'cloudfront',
    profile,
    region,
    cloudfrontFromRaw,
  );
  const row = useMemo(() => data?.find((r) => r.id === id), [data, id]);
  const rows = row?.behaviors ?? [];

  return (
    <div className="section">
      <h3>Behaviors{row ? ` (${rows.length})` : ''}</h3>
      {/* 一覧未取得の間は該当行の有無を断定せずローディングを表示する (issues/0085 と同じ規則)。 */}
      {!row && listLoading ? (
        <DrawerLoading />
      ) : rows.length === 0 ? (
        <p className="muted">No behaviors.</p>
      ) : (
        <DataTable
          rows={rows}
          columns={cloudfrontBehaviorColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </div>
  );
}
