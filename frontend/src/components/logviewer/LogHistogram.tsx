// ログビューアのヒストグラム表示。buildHistogram (lib/logHistogram.ts) が作ったバケット列を
// 棒グラフで描く。mode='mono' は単色 + severity で色分け (CloudWatch)、'stacked' は
// severity 積み上げ (Cloud Logging)。
import { useTranslation } from 'react-i18next';
import { type HistogramBucket, dominantLevel } from '../../lib/logHistogram';

export interface LogHistogramProps {
  buckets: HistogramBucket[];
  mode: 'mono' | 'stacked';
}

export function LogHistogram({ buckets, mode }: LogHistogramProps) {
  const { t } = useTranslation('logviewer');
  const max = Math.max(1, ...buckets.map((b) => b.total));

  return (
    <div className="lv-histogram">
      {buckets.map((b, i) => {
        if (mode === 'mono') {
          const level = dominantLevel(b);
          const h = (b.total / max) * 100;
          return (
            <div key={i} className="lv-hcol" title={t('logHistogram.bucketTotal', { n: b.total })}>
              <div className={`lv-hbar lv-hbar-${level}`} style={{ height: `${h}%` }} />
            </div>
          );
        }
        return (
          <div
            key={i}
            className="lv-hcol lv-hcol-stack"
            title={`ERROR ${b.err} / WARNING ${b.warn} / INFO ${b.info}`}
          >
            <div className="lv-hseg lv-hseg-err" style={{ height: `${(b.err / max) * 100}%` }} />
            <div className="lv-hseg lv-hseg-warn" style={{ height: `${(b.warn / max) * 100}%` }} />
            <div className="lv-hseg lv-hseg-info" style={{ height: `${(b.info / max) * 100}%` }} />
          </div>
        );
      })}
    </div>
  );
}
