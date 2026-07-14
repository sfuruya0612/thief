// BigQuery ビュー: dataset 一覧 → table 一覧 → schema 表示 + アドホック SQL 実行
// GCP 統合ビューの一部として projectId prop を受け取り、全 BQ フックへ流し込む
import { useState } from 'react';
import { useBQDatasets, useBQQuery, useBQSchema, useBQTables } from '../../api/queries';
import { DataTable } from '../../components/DataTable';
import { bqFieldColumns, bqTableColumns } from '../../components/tables/nonAwsColumns';
import { Icons } from '../../components/icons/Icons';
import { ErrorBanner } from '../../components/ErrorBanner';

export interface BigQueryViewProps {
  projectId?: string;
}

export function BigQueryView({ projectId }: BigQueryViewProps = {}) {
  const [selectedDataset, setSelectedDataset] = useState<string | null>(null);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [sql, setSql] = useState('SELECT 1');

  const { data: datasets, error: datasetsError } = useBQDatasets(projectId);
  const { data: tables, error: tablesError } = useBQTables(selectedDataset ?? '', projectId);
  const { data: fields, error: schemaError } = useBQSchema(
    selectedDataset ?? '',
    selectedTable ?? '',
    projectId,
  );
  const runQuery = useBQQuery();

  const resourceError = datasetsError ?? tablesError ?? schemaError;

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>BigQuery</h1>
          <span className="subtitle">datasets & tables</span>
        </div>
      </div>

      {resourceError && <ErrorBanner error={resourceError} />}

      <div className="query-box">
        <textarea value={sql} onChange={(e) => setSql(e.target.value)} spellCheck={false} />
        <div className="query-box-actions">
          <button
            className="btn sm primary"
            disabled={runQuery.isPending || !sql.trim()}
            onClick={() => runQuery.mutate({ sql, projectId })}
          >
            {runQuery.isPending ? 'Running…' : 'Run query'}
          </button>
          {runQuery.isError && (
            <span className="query-error">{(runQuery.error as Error).message}</span>
          )}
        </div>
        {runQuery.data && (
          <div className="table-wrap">
            <table className="dt">
              <thead>
                <tr>
                  {runQuery.data.columns.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {runQuery.data.rows.map((row, i) => (
                  <tr key={i}>
                    {row.map((cell, j) => (
                      <td key={j} style={{ fontFamily: 'var(--font-mono)' }}>
                        {cell}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="nonaws-cols">
        <div className="nonaws-list">
          {(datasets ?? []).map((d) => (
            <div
              key={d.id}
              className={`nav-item ${selectedDataset === d.id ? 'active' : ''}`}
              onClick={() => {
                setSelectedDataset(d.id);
                setSelectedTable(null);
              }}
            >
              <Icons.s3 size={14} />
              <span className="truncate">{d.name}</span>
            </div>
          ))}
          {(datasets ?? []).length === 0 && <div className="empty-hint">No datasets</div>}
        </div>

        {!selectedTable ? (
          <DataTable
            rows={tables ?? []}
            columns={bqTableColumns}
            onSelect={(t) => setSelectedTable(t.id)}
            selectedId={selectedTable}
          />
        ) : (
          <div className="col" style={{ minHeight: 0, overflow: 'hidden' }}>
            <div style={{ padding: '8px 16px' }}>
              <button className="btn sm ghost" onClick={() => setSelectedTable(null)}>
                ← Back to tables
              </button>
            </div>
            <DataTable
              rows={fields ?? []}
              columns={bqFieldColumns}
              onSelect={() => {}}
              selectedId={null}
            />
          </div>
        )}
      </div>
    </div>
  );
}
