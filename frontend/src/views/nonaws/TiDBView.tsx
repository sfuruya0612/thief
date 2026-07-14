// TiDB ビュー: project 一覧 → cluster 一覧 + cost 表示
import { useState } from 'react';
import { useTiDBClusters, useTiDBCost, useTiDBProjects } from '../../api/queries';
import { DataTable } from '../../components/DataTable';
import { tidbClusterColumns, tidbCostColumns } from '../../components/tables/nonAwsColumns';
import { ErrorBanner } from '../../components/ErrorBanner';

type Tab = 'clusters' | 'cost';

export function TiDBView() {
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('clusters');

  const { data: projects, error: projectsError } = useTiDBProjects();
  const { data: clusters, error: clustersError } = useTiDBClusters(selectedProject ?? '');
  const { data: cost, error: costError } = useTiDBCost();

  const error = tab === 'cost' ? costError : (projectsError ?? clustersError);

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
        <DataTable
          rows={cost ?? []}
          columns={tidbCostColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      ) : (
        <div className="nonaws-cols">
          <div className="nonaws-list">
            {(projects ?? []).map((p) => (
              <div
                key={p.id}
                className={`nav-item ${selectedProject === p.id ? 'active' : ''}`}
                onClick={() => setSelectedProject(p.id)}
              >
                <span className="truncate">{p.name}</span>
                <span className="meta">{p.clusterCount}</span>
              </div>
            ))}
            {(projects ?? []).length === 0 && <div className="empty-hint">No projects</div>}
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
