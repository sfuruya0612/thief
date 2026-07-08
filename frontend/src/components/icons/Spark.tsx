// icons.jsx Spark を TSX 化

export interface SparkProps {
  values: number[];
  w?: number;
  h?: number;
  stroke?: string;
  fill?: string;
}

export function Spark({
  values,
  w = 60,
  h = 16,
  stroke = 'currentColor',
  fill = 'none',
}: SparkProps) {
  if (!values || values.length === 0) return null;
  const step = w / (values.length - 1);
  const pts = values.map((v, i) => `${(i * step).toFixed(1)},${(h - v * h).toFixed(1)}`).join(' ');
  const area = `0,${h} ${pts} ${w},${h}`;
  return (
    <svg
      width={w}
      height={h}
      viewBox={`0 0 ${w} ${h}`}
      style={{ display: 'block', overflow: 'visible' }}
    >
      {fill !== 'none' && <polygon points={area} fill={fill} />}
      <polyline
        points={pts}
        fill="none"
        stroke={stroke}
        strokeWidth="1.25"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}
