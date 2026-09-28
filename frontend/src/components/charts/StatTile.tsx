// query_value ウィジェット用の単一値カード。描画は primitives の Stat に任せ、
// 値の書式 (丸め) と hover の文言だけをこのコンポーネントが決める。
import { Stat } from '../primitives';

export interface StatTileProps {
  title: string;
  // 値が取れていない場合は null (取得前・欠測)。
  value: number | null;
  unit?: string;
  // 小数点以下の桁数。既定は 2 桁。
  precision?: number;
}

// DASH は値が無いことを示す記号。0 と区別する。
const DASH = '—';

export function StatTile({ title, value, unit, precision = 2 }: StatTileProps) {
  const display =
    value === null ? DASH : value.toLocaleString(undefined, { maximumFractionDigits: precision });
  // ホバーで丸めていない値を読めるようにする。桁を落とした表示だけでは、丸めの前後で
  // 意味が変わる値 (しきい値付近など) を確かめられない。
  const hover = value === null ? title : `${title}: ${value}${unit ? ` ${unit}` : ''}`;

  return <Stat label={title} value={display} unit={unit || undefined} title={hover} />;
}
