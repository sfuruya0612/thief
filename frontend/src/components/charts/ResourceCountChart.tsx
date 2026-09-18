// 台数の推移を描く折れ線グラフ。EC2 は Running インスタンス数、ECS はクラスタごとの
// タスク数を 1 本ずつの系列として表示する。
//
// 取得は一覧 (useResources) とは別の useQuery で行うため、一覧の表示は時系列の取得完了を
// 待たない。取得中はグラフ領域に読み込み中を出し、失敗したときは理由だけを出して
// グラフ領域は出さない (軸だけのグラフは「値が 0 だった」と読めてしまう)。
import { useState } from 'react';
import { TimeseriesChart } from './TimeseriesChart';
import { ErrorBanner } from '../ErrorBanner';
import { useResourceTimeseries } from '../../api/queries';
import type { TimeseriesRange } from '../../types/aws';

export interface ResourceCountChartProps {
  // 一覧と同じサービスキー (ec2 / ecs)。
  service: string;
  profile: string;
  region: string;
  // グラフの見出し。何を数えた値なのかを示す。
  title: string;
}

// RANGE_OPTIONS は切り替えられる期間。粒度は backend が期間から決めるため、
// ここでは窓の長さだけを選ばせる。
const RANGE_OPTIONS: { label: string; value: TimeseriesRange }[] = [
  { label: '1 day', value: '1d' },
  { label: '7 days', value: '7d' },
  { label: '1 month', value: '30d' },
];

const CHART_HEIGHT = 220;

// DEFAULT_RANGE は開いた直後に表示する期間。EC2 の台数は一覧の取得時にだけ記録されるため、
// 1 日の窓では点が少なく推移が読めない。7 日を既定にして直近の推移が入るようにする。
const DEFAULT_RANGE: TimeseriesRange = '7d';

export function ResourceCountChart({ service, profile, region, title }: ResourceCountChartProps) {
  const [range, setRange] = useState<TimeseriesRange>(DEFAULT_RANGE);
  const { data, isLoading, error } = useResourceTimeseries(service, profile, region, range);

  return (
    <div className="stats" style={{ gridTemplateColumns: '1fr' }}>
      <div className="stat">
        <div className="timeseries-head">
          <div className="label">{title}</div>
          <div className="seg">
            {RANGE_OPTIONS.map((o) => (
              <button
                key={o.value}
                className={range === o.value ? 'active' : ''}
                onClick={() => setRange(o.value)}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>

        {error ? (
          <ErrorBanner error={error} />
        ) : isLoading ? (
          <div className="empty-hint" style={{ height: CHART_HEIGHT }}>
            Loading timeseries…
          </div>
        ) : (
          // X 軸の範囲は応答の時間窓に合わせる。点の範囲から決めると、記録された点が
          // 少ない EC2 の台数では期間を切り替えても軸が変わらない。
          // 窓が正の幅を持たない応答 (start / end を返さない古い backend など) では渡さず、
          // 軸が 0 に潰れて全系列が消えるのを避ける。
          <TimeseriesChart
            series={data?.series ?? []}
            height={CHART_HEIGHT}
            xRange={
              data && data.end > data.start ? { start: data.start, end: data.end } : undefined
            }
          />
        )}
      </div>
    </div>
  );
}
