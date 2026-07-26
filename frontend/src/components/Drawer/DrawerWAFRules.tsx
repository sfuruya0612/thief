// Web ACL のルール一覧を表示する Drawer の Rules タブ。
// Drawer に渡る resource (BaseRow) は scope を持たないため、一覧クエリ (useResources) の
// キャッシュから id で該当行を引き直して scope を得る (REGIONAL と CLOUDFRONT に同名の
// Web ACL が存在しうるため name では引かない)。行がまだ無い間は取得を無効化して
// ローディング表示を出す。
import { useMemo } from 'react';
import { useResources, useWAFRules } from '../../api/queries';
import { wafFromRaw } from '../../lib/normalize';
import { wafRuleColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import type { WAFRaw, WAFRow } from '../../types/aws';

export interface DrawerWAFRulesProps {
  profile: string;
  region: string;
  id: string;
  name: string;
}

export function DrawerWAFRules({ profile, region, id, name }: DrawerWAFRulesProps) {
  const { data: acls } = useResources<WAFRaw, WAFRow>('waf', profile, region, wafFromRaw);
  const scope = useMemo(() => acls?.find((r) => r.id === id)?.scope ?? '', [acls, id]);

  const { data, isLoading, error } = useWAFRules(profile, region, scope, name, id);
  const rules = useMemo(() => data ?? [], [data]);

  // 一覧キャッシュから scope が引けるまで、および取得中はローディング表示
  // (0 件の空表示ともエラー表示とも区別する)。
  if (!scope || isLoading) {
    return (
      <div className="section">
        <h3>Rules</h3>
        <DrawerLoading />
      </div>
    );
  }

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す。
  return (
    <div className="section">
      <h3>Rules ({rules.length})</h3>
      {error != null && <DrawerError error={error} />}
      {data !== undefined && (
        <DataTable rows={rules} columns={wafRuleColumns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}
