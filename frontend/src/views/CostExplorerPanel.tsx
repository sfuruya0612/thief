// Cost Explorer 専用パネル。ServicePanel (汎用 15 サービス共通) とは異なり、
// リソース一覧ではなくコストの chart + クロス集計テーブルを表示するため専用実装とする。
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCost } from '../api/queries';
import { CostChart } from '../components/charts/CostChart';
import { CostCrossTable } from '../components/tables/CostCrossTable';
import { Icons } from '../components/icons/Icons';
import { Loading } from '../components/Loading';
import { isSSOExpiredError } from '../lib/ssoError';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';
import { ErrorBanner } from '../components/ErrorBanner';
import { aggregateCost, type CostMetricType } from '../lib/costAggregate';

export interface CostExplorerPanelProps {
  profile: string;
  region: string;
}

const GRANULARITY_OPTIONS = [
  { value: 'DAILY', label: 'Daily' },
  { value: 'MONTHLY', label: 'Monthly' },
];

const GROUP_BY_OPTIONS = [
  { value: 'SERVICE', label: 'Service' },
  { value: 'USAGE_TYPE', label: 'Usage type' },
  { value: 'LINKED_ACCOUNT', label: 'Linked account' },
  { value: 'REGION', label: 'Region' },
];

const METRIC_OPTIONS: { value: CostMetricType; label: string }[] = [
  { value: 'unblended', label: 'Unblended' },
  { value: 'netAmortized', label: 'Net Amortized' },
];

// 期間ショートカット。クリックすると開始/終了日入力を一括で埋める (それ以降は日付入力を
// 自由に編集できる。API 呼び出しは開始/終了日の確定値に対してのみ行われる)。
const RANGE_PRESETS = [
  { labelKey: 'costExplorerPanel.presets.last1Week', days: 7 },
  { labelKey: 'costExplorerPanel.presets.last1Month', days: 30 },
  { labelKey: 'costExplorerPanel.presets.last3Months', days: 90 },
  { labelKey: 'costExplorerPanel.presets.last6Months', days: 180 },
];

function toDateInputValue(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function defaultDateRange(): { start: string; end: string } {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - 30);
  return { start: toDateInputValue(start), end: toDateInputValue(end) };
}

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる
const MAX_SERIES = 8;

interface CostFilterInputProps {
  value: string;
  onChange: (v: string) => void;
  onCommit: (v: string) => void;
  placeholder: string;
  title: string;
}

// CostFilterInput は Cost Explorer のフィルタ入力欄。GetCostAndUsage は有償 API のため
// 入力の 1 文字ごとには確定させず、Enter の押下または入力欄からのフォーカス離脱で確定させる。
function CostFilterInput({ value, onChange, onCommit, placeholder, title }: CostFilterInputProps) {
  // 確定時は前後の空白を除いた値を確定値とし、入力欄の表示値も同じ値に揃える。表示値だけ
  // 空白付きで残すと、実際に絞り込みへ使われる値と画面の表示が食い違って見える。
  const commit = () => {
    const trimmed = value.trim();
    if (trimmed !== value) onChange(trimmed);
    onCommit(trimmed);
  };

  return (
    <span className="chip-search">
      <Icons.search size={12} />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            commit();
          }
        }}
        onBlur={commit}
        placeholder={placeholder}
        title={title}
      />
    </span>
  );
}

export function CostExplorerPanel({ profile, region }: CostExplorerPanelProps) {
  const { t } = useTranslation('cost');
  const initialRange = useMemo(defaultDateRange, []);
  const [granularity, setGranularity] = useState('DAILY');
  const [groupBy, setGroupBy] = useState('SERVICE');
  const [startDate, setStartDate] = useState(initialRange.start);
  const [endDate, setEndDate] = useState(initialRange.end);
  const [metric, setMetric] = useState<CostMetricType>('unblended');

  // 絞り込みは 1 つのキーワードで行い、backend が Service / Usage type / Linked account の
  // 3 次元を横断して部分一致で解決する。GetCostAndUsage はリクエストごとに課金される API の
  // ため、入力中の値 (keywordInput) と API に渡す確定値 (keywordApplied) を分離し、Enter の
  // 押下かフォーカス離脱で確定させたときにのみ API を呼び出す。
  const [keywordInput, setKeywordInput] = useState('');
  const [keywordApplied, setKeywordApplied] = useState('');

  const { data, isLoading, error } = useCost(profile, region, {
    granularity,
    groupBy,
    keyword: keywordApplied,
    startDate,
    endDate,
  });

  const ssoExpired = isSSOExpiredError(error);
  const rows = useMemo(() => data ?? [], [data]);

  const { categories, series, crossTableRows, total } = useMemo(
    () => aggregateCost(rows, metric, MAX_SERIES),
    [rows, metric],
  );

  const applyPreset = (days: number) => {
    const end = new Date();
    const start = new Date(end);
    start.setDate(start.getDate() - days);
    setStartDate(toDateInputValue(start));
    setEndDate(toDateInputValue(end));
  };

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>Cost Explorer</h1>
          <span className="subtitle">cost &amp; usage</span>
        </div>
      </div>

      {ssoExpired && <SSOExpiredBanner profile={profile} />}
      {!ssoExpired && error && <ErrorBanner error={error} />}

      <div className="stats" style={{ gridTemplateColumns: 'repeat(1, 1fr)' }}>
        <div className="stat">
          <div className="label">
            Total ({METRIC_OPTIONS.find((o) => o.value === metric)?.label})
          </div>
          <div className="value">
            ${total.toLocaleString(undefined, { maximumFractionDigits: 2 })}
          </div>
          <div className="delta"> </div>
        </div>
      </div>

      <div className="facets">
        <CostFilterInput
          value={keywordInput}
          onChange={setKeywordInput}
          onCommit={setKeywordApplied}
          placeholder="filter by service / usage type / account…"
          title="Filter by service name, usage type, account ID or name (press Enter to apply)"
        />

        <input
          type="date"
          className="btn sm"
          value={startDate}
          max={endDate}
          onChange={(e) => setStartDate(e.target.value)}
          title="Start date"
        />
        <span style={{ color: 'var(--text-3)' }}>–</span>
        <input
          type="date"
          className="btn sm"
          value={endDate}
          min={startDate}
          onChange={(e) => setEndDate(e.target.value)}
          title="End date"
        />

        {RANGE_PRESETS.map((p) => (
          <button
            key={p.labelKey}
            className="btn sm ghost"
            onClick={() => applyPreset(p.days)}
            title={t(p.labelKey)}
          >
            {t(p.labelKey)}
          </button>
        ))}

        <select
          className="btn sm"
          value={granularity}
          onChange={(e) => setGranularity(e.target.value)}
          title="Granularity"
        >
          {GRANULARITY_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>

        <select
          className="btn sm"
          value={groupBy}
          onChange={(e) => setGroupBy(e.target.value)}
          title="Group by"
        >
          {GROUP_BY_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>

        <select
          className="btn sm"
          value={metric}
          onChange={(e) => setMetric(e.target.value as CostMetricType)}
          title="Cost metric"
        >
          {METRIC_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      {isLoading ? (
        <Loading />
      ) : (
        <>
          <CostChart categories={categories} series={series} />
          <CostCrossTable categories={categories} rows={crossTableRows} />
        </>
      )}
    </div>
  );
}
