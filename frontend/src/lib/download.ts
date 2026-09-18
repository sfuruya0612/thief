// 集計表を CSV ファイルとして保存するヘルパー。
// 文字列生成は lib/queryFormat.ts の toCsv に委ね、ここでは Blob と <a download> の
// 生成だけを行う。
import { toCsv } from './queryFormat';

// CrossTableCsvRow は CostCrossTable の行と同じ形。コンポーネントへの依存を避けるため
// 構造的に定義する。
export interface CrossTableCsvRow {
  group: string;
  amounts: number[];
  total: number;
}

export interface CsvTable {
  columns: string[];
  rows: string[][];
}

// crossTableCsv はクロス集計表 (Group / Total / 期間列) を CSV の列と行に変換する。
// 金額は桁区切りを外した小数 2 桁固定にして、表計算ソフトで数値として扱えるようにする。
export function crossTableCsv(categories: string[], rows: CrossTableCsvRow[]): CsvTable {
  const columns = ['Group', 'Total', ...categories];
  const csvRows = rows.map((r) => [
    r.group,
    r.total.toFixed(2),
    ...r.amounts.map((a) => a.toFixed(2)),
  ]);
  return { columns, rows: csvRows };
}

// downloadCsv は CSV 文字列を Blob にしてファイルとしてダウンロードさせる。
export function downloadCsv(filename: string, table: CsvTable): void {
  const csv = toCsv(table.columns, table.rows);
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
