// TiDB ビュー: project 一覧 → cluster 一覧 + cost 表示 (AWS Cost Explorer と同じ chart + クロス集計表)
import { useMemo, useState } from 'react';
import { useTiDBClusters, useTiDBCost, useTiDBProjects } from '../../api/queries';
import { DataTable } from '../../components/DataTable';
import { CostChart } from '../../components/charts/CostChart';
import { CostCrossTable } from '../../components/tables/CostCrossTable';
import { Loading } from '../../components/Loading';
import { Icons } from '../../components/icons/Icons';
import { tidbClusterColumns } from '../../components/tables/nonAwsColumns';
import { ErrorBanner } from '../../components/ErrorBanner';
import { aggregateTiDBCost, type TiDBCostGroupBy } from '../../lib/costAggregateTiDB';

type Tab = 'clusters' | 'cost';

const GROUP_BY_OPTIONS: { value: TiDBCostGroupBy; label: string }[] = [
  { value: 'servicePathName', label: 'Service' },
  { value: 'projectName', label: 'Project' },
  { value: 'clusterName', label: 'Cluster' },
];

// TiDB Cloud の billing API は月単位でしか取得できないため、Cost Explorer の日付範囲では
// なく年月 (YYYY-MM) の範囲で期間を指定する。
const RANGE_PRESETS = [
  { label: '直近 3 ヶ月', months: 3 },
  { label: '直近 6 ヶ月', months: 6 },
  { label: '直近 12 ヶ月', months: 12 },
];

function toMonthInputValue(d: Date): string {
  return d.toISOString().slice(0, 7);
}

function defaultMonthRange(): { start: string; end: string } {
  const end = new Date();
  const start = new Date(end);
  start.setMonth(start.getMonth() - 2);
  return { start: toMonthInputValue(start), end: toMonthInputValue(end) };
}

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる
const MAX_SERIES = 8;

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

  // API 呼び出しは期間のみに依存させる。グループ名フィルタは取得済みデータに対して
  // ブラウザ側で絞り込むだけにし、都度 API を呼び出さない (AWS Cost Explorer と同じ方式)。
  const allCostRows = useMemo(() => cost ?? [], [cost]);
  const filteredCostRows = useMemo(() => {
    if (!groupFilter.trim()) return allCostRows;
    const needle = groupFilter.trim().toLowerCase();
    return allCostRows.filter((r) => r[groupBy].toLowerCase().includes(needle));
  }, [allCostRows, groupFilter, groupBy]);

  const { categories, series, crossTableRows, total } = useMemo(
    () => aggregateTiDBCost(filteredCostRows, groupBy, MAX_SERIES),
    [filteredCostRows, groupBy],
  );

  const applyPreset = (months: number) => {
    const end = new Date();
    const start = new Date(end);
    start.setMonth(start.getMonth() - (months - 1));
    setStartMonth(toMonthInputValue(start));
    setEndMonth(toMonthInputValue(end));
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
        <>
          <div className="stats" style={{ gridTemplateColumns: 'repeat(1, 1fr)' }}>
            <div className="stat">
              <div className="label">Total</div>
              <div className="value">
                ${total.toLocaleString(undefined, { maximumFractionDigits: 2 })}
              </div>
              <div className="delta"> </div>
            </div>
          </div>

          <div className="facets">
            <span className="chip-search">
              <Icons.search size={12} />
              <input
                value={groupFilter}
                onChange={(e) => setGroupFilter(e.target.value)}
                placeholder={`filter by ${GROUP_BY_OPTIONS.find((o) => o.value === groupBy)?.label.toLowerCase()} name (client-side)…`}
              />
            </span>

            <input
              type="month"
              className="btn sm"
              value={startMonth}
              max={endMonth}
              onChange={(e) => setStartMonth(e.target.value)}
              title="Start month"
            />
            <span style={{ color: 'var(--text-3)' }}>–</span>
            <input
              type="month"
              className="btn sm"
              value={endMonth}
              min={startMonth}
              onChange={(e) => setEndMonth(e.target.value)}
              title="End month"
            />

            {RANGE_PRESETS.map((p) => (
              <button
                key={p.label}
                className="btn sm ghost"
                onClick={() => applyPreset(p.months)}
                title={p.label}
              >
                {p.label}
              </button>
            ))}

            <select
              className="btn sm"
              value={groupBy}
              onChange={(e) => {
                setGroupBy(e.target.value as TiDBCostGroupBy);
                setGroupFilter('');
              }}
              title="Group by"
            >
              {GROUP_BY_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>

          {costLoading ? (
            <Loading />
          ) : (
            <>
              <CostChart categories={categories} series={series} />
              <CostCrossTable categories={categories} rows={crossTableRows} />
            </>
          )}
        </>
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
