// Datadog ビュー: historical / estimated cost の切替表示
import { useState } from 'react';
import { useDatadogEstimated, useDatadogHistorical } from '../../api/queries';
import { DataTable } from '../../components/DataTable';
import { datadogCostColumns } from '../../components/tables/nonAwsColumns';

type Mode = 'historical' | 'estimated';

export function DatadogView() {
  const [mode, setMode] = useState<Mode>('historical');
  const { data: historical } = useDatadogHistorical();
  const { data: estimated } = useDatadogEstimated();

  const rows = mode === 'historical' ? (historical ?? []) : (estimated ?? []);

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>Datadog</h1>
          <span className="subtitle">cost</span>
        </div>
        <div className="seg" style={{ width: 200 }}>
          <button
            className={mode === 'historical' ? 'active' : ''}
            onClick={() => setMode('historical')}
          >
            Historical
          </button>
          <button
            className={mode === 'estimated' ? 'active' : ''}
            onClick={() => setMode('estimated')}
          >
            Estimated
          </button>
        </div>
      </div>

      <DataTable rows={rows} columns={datadogCostColumns} onSelect={() => {}} selectedId={null} />
    </div>
  );
}
