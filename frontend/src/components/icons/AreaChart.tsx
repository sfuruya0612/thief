// icons.jsx AreaChart を TSX 化

export interface AreaChartProps {
  values: number[];
  h?: number;
  stroke?: string;
  fill?: string;
  fillOpacity?: number;
}

export function AreaChart({
  values,
  h = 48,
  stroke = 'currentColor',
  fill = 'currentColor',
  fillOpacity = 0.12,
}: AreaChartProps) {
  if (!values || values.length === 0) return null;
  const w = 100;
  const step = w / (values.length - 1);
  const pts = values.map((v, i) => `${(i * step).toFixed(2)},${(h - v * h).toFixed(2)}`).join(' ');
  return (
    <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{ width: '100%', height: h }}>
      <polygon points={`0,${h} ${pts} ${w},${h}`} fill={fill} opacity={fillOpacity} />
      <polyline
        points={pts}
        fill="none"
        stroke={stroke}
        strokeWidth="1"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
