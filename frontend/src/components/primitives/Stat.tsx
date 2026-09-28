// 統計カード (primitives.css の .stat): ラベル、値 (+ 単位)、差分の 3 段 (docs/issues/closed/0203)。
// グラフやウィジェットを載せるカード (Datadog のビュー、ResourceCountChart) は形が違うため
// この部品を使わず、.stat クラスをそのまま使う。
import type { ReactNode } from 'react';

export interface StatProps {
  label: ReactNode;
  value: ReactNode;
  // 値の右に小さく添える単位 (.stat .value .unit)。
  unit?: ReactNode;
  // 3 段目 (.stat .delta)。省略すると描画しない。高さをそろえるためだけに置くときは ' ' を渡す。
  delta?: ReactNode;
  // delta の色 (pos = --ok、neg = --err-ink)。
  tone?: 'pos' | 'neg';
  title?: string;
}

export function Stat({ label, value, unit, delta, tone, title }: StatProps) {
  return (
    <div className="stat" title={title}>
      <div className="label">{label}</div>
      <div className="value">
        {value}
        {unit !== undefined && <span className="unit">{unit}</span>}
      </div>
      {delta !== undefined && <div className={tone ? `delta ${tone}` : 'delta'}>{delta}</div>}
    </div>
  );
}
