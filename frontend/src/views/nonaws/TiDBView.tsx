// TiDB ビュー: project 一覧 → cluster 一覧 + cost 表示 (AWS Cost Explorer と同じ chart + クロス集計表)
import { useMemo, useState } from 'react';
import { useTiDBClusters, useTiDBCost, useTiDBProjects } from '../../api/queries';
import { DataTable } from '../../components/DataTable';
import { MonthlyCostPanel } from '../../components/MonthlyCostPanel';
import { Icons } from '../../components/icons/Icons';
import { tidbClusterColumns } from '../../components/tables/nonAwsColumns';
import { ErrorBanner } from '../../components/ErrorBanner';
import { aggregateTiDBCost, type TiDBCostGroupBy } from '../../lib/costAggregateTiDB';
import { defaultMonthRange, lastMonthsRange } from '../../lib/monthRange';
import type { TiDBCostRow } from '../../types/nonaws';

type Tab = 'clusters' | 'cost';

const GROUP_BY_OPTIONS: { value: TiDBCostGroupBy; label: string }[] = [
  { value: 'servicePathName', label: 'Service' },
  { value: 'projectName', label: 'Project' },
  { value: 'clusterName', label: 'Cluster' },
];

function groupValueOf(r: TiDBCostRow, groupBy: TiDBCostGroupBy): string {
  return r[groupBy];
}

// TiDB Cloud の billing API は月単位でしか取得できないため、Cost Explorer の日付範囲では
// なく年月 (YYYY-MM) の範囲で期間を指定する。
export function TiDBView() {
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('clusters');
  const [projectFilter, setProjectFilter] = useState('');
  const [projectSortDir, setProjectSortDir] = useState<'asc' | 'desc'>('asc');

  const initialRange = useMemo(defaultMonthRange, []);
  const [startMonth, setStartMonth] = useState(initialRange.start);
  const [endMonth, setEndMonth] = useState(initialRange.end);
  const [groupBy, setGroupBy] = useState<TiDBCostGroupBy>('servicePathName');
  const [groupFilter, setGroupFilter] = useState('');

  const { data: projects, error: projectsError } = useTiDBProjects();
  const { data: clusters, error: clustersError } = useTiDBClusters(selectedProject ?? '');

  // プロジェクト名での検索フィルタ + ソート (取得済みデータに対するブラウザ側処理)。
  const visibleProjects = useMemo(() => {
    const needle = projectFilter.trim().toLowerCase();
    const filtered = needle
      ? (projects ?? []).filter((p) => p.name.toLowerCase().includes(needle))
      : (projects ?? []);
    const sorted = [...filtered].sort((a, b) => a.name.localeCompare(b.name));
    return projectSortDir === 'asc' ? sorted : sorted.reverse();
  }, [projects, projectFilter, projectSortDir]);
  const {
    data: cost,
    error: costError,
    isLoading: costLoading,
  } = useTiDBCost({ start: startMonth, end: endMonth });

  const error = tab === 'cost' ? costError : (projectsError ?? clustersError);

  // API 呼び出しは期間のみに依存させる。グループ名フィルタは MonthlyCostPanel 側で
  // 取得済みデータに対して絞り込むだけにし、都度 API を呼び出さない。
  const allCostRows = useMemo(() => cost ?? [], [cost]);

  const applyPreset = (months: number) => {
    const range = lastMonthsRange(months);
    setStartMonth(range.start);
    setEndMonth(range.end);
  };

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>TiDB Cloud</h1>
          <span className="subtitle">projects & clusters</span>
        </div>
        <div className="seg" style={{ width: 200 }}>
          <button className={tab === 'clusters' ? 'active' : ''} onClick={() => setTab('clusters')}>
            Clusters
          </button>
          <button className={tab === 'cost' ? 'active' : ''} onClick={() => setTab('cost')}>
            Cost
          </button>
        </div>
      </div>

      {error && <ErrorBanner error={error} />}

      {tab === 'cost' ? (
        <MonthlyCostPanel
          rows={allCostRows}
          isLoading={costLoading}
          groupByOptions={GROUP_BY_OPTIONS}
          groupBy={groupBy}
          onGroupByChange={(g) => {
            setGroupBy(g);
            setGroupFilter('');
          }}
          groupFilter={groupFilter}
          onGroupFilterChange={setGroupFilter}
          startMonth={startMonth}
          endMonth={endMonth}
          onStartMonthChange={setStartMonth}
          onEndMonthChange={setEndMonth}
          onApplyPreset={applyPreset}
          aggregate={aggregateTiDBCost}
          groupValueOf={groupValueOf}
        />
      ) : (
        <div className="nonaws-cols">
          <div className="nonaws-list-pane">
            <div className="nonaws-list-filter">
              <span className="chip-search">
                <Icons.search size={12} />
                <input
                  value={projectFilter}
                  onChange={(e) => setProjectFilter(e.target.value)}
                  placeholder="filter by project name…"
                />
              </span>
              <button
                className="btn sm ghost"
                onClick={() => setProjectSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))}
                title={projectSortDir === 'asc' ? 'Sort: A → Z' : 'Sort: Z → A'}
              >
                <Icons.chevron
                  size={12}
                  style={{
                    transform: projectSortDir === 'asc' ? 'rotate(-90deg)' : 'rotate(90deg)',
                  }}
                />
              </button>
            </div>
            <div className="nonaws-list">
              {visibleProjects.map((p) => (
                <div
                  key={p.id}
                  className={`nav-item ${selectedProject === p.id ? 'active' : ''}`}
                  onClick={() => setSelectedProject(p.id)}
                >
                  <span className="truncate">{p.name}</span>
                </div>
              ))}
              {visibleProjects.length === 0 && <div className="empty-hint">No projects</div>}
            </div>
          </div>

          {selectedProject ? (
            <DataTable
              rows={clusters ?? []}
              columns={tidbClusterColumns}
              onSelect={() => {}}
              selectedId={null}
            />
          ) : (
            <div className="empty-hint">Select a project</div>
          )}
        </div>
      )}
    </div>
  );
}
