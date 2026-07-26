// Web ACL のルール一覧を表示する Drawer の Rules タブ。
// Drawer に渡る resource (BaseRow) は scope を持たないため、一覧クエリ (useResources) の
// キャッシュから id で該当行を引き直して scope を得る (REGIONAL と CLOUDFRONT に同名の
// Web ACL が存在しうるため name では引かない)。行がまだ無い間は取得を無効化して
// ローディング表示を出す。
//
// 行を選択すると、一覧の下にそのルールの ruleJson (Statement を含むルール定義全体)
// を整形して併置表示する。差し替え方式 (DrawerObjectBrowser) ではなく併置を採るのは、
// 行を切り替えながら JSON を見比べる用途で一覧が見え続ける必要があるため。
import { useMemo, useState } from 'react';
import { useResources, useWAFRules } from '../../api/queries';
import { wafFromRaw } from '../../lib/normalize';
import { wafRuleColumns } from '../tables/columns';
import { Dash } from '../tables/cells';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import type { WAFRaw, WAFRow, WAFRuleRow } from '../../types/aws';

export interface DrawerWAFRulesProps {
  profile: string;
  region: string;
  id: string;
  name: string;
}

// selectedRule.ruleJson を整形して表示する。空文字はダッシュ、JSON として不正な
// 場合は文字列をそのまま表示する (backend が json.Marshal の出力か空文字のみを
// 返す契約の下では通常到達しない防御)。
function RuleJSONDetail({ ruleJson }: { ruleJson: string }) {
  if (ruleJson === '') return <Dash />;
  let formatted = ruleJson;
  try {
    formatted = JSON.stringify(JSON.parse(ruleJson), null, 2);
  } catch {
    // パースできない JSON は生文字列のまま表示する
  }
  return <pre className="logbox">{formatted}</pre>;
}

export function DrawerWAFRules({ profile, region, id, name }: DrawerWAFRulesProps) {
  const { data: acls } = useResources<WAFRaw, WAFRow>('waf', profile, region, wafFromRaw);
  const scope = useMemo(() => acls?.find((r) => r.id === id)?.scope ?? '', [acls, id]);

  const { data, isLoading, error } = useWAFRules(profile, region, scope, name, id);
  const rules = useMemo(() => data ?? [], [data]);

  const [selectedName, setSelectedName] = useState<string | null>(null);
  const selectedRule = useMemo(
    () => rules.find((r) => r.name === selectedName) ?? null,
    [rules, selectedName],
  );

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
        <DataTable
          rows={rules}
          columns={wafRuleColumns}
          onSelect={(r: WAFRuleRow) => setSelectedName(r.name)}
          selectedId={selectedName}
        />
      )}
      {selectedRule && (
        <>
          <h3>{selectedRule.name}</h3>
          <RuleJSONDetail ruleJson={selectedRule.ruleJson} />
        </>
      )}
    </div>
  );
}
