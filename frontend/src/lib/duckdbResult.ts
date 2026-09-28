// DuckDB のクエリ結果 (Arrow のレコードバッチ) を ResultTable が要求する
// columns / rows (string[][]) へ変換する純関数。Arrow の値は型ごとに次の規則で文字列にする。
// - null / undefined は空文字列
// - boolean は true / false
// - 整数 (BigInt を含む) と Decimal は 10 進の文字列
// - Date / Timestamp は ISO 8601 の文字列 (UTC)
// - それ以外 (文字列、リスト、構造体など) は String(value)
//
// Arrow の get() は Date / Timestamp をエポックミリ秒の数値で返す (arrow 17 の visitor)。
// 数値のままでは整数と区別できないため、列の型が日付・時刻の場合は ISO 8601 へ変換する。
import { Type } from 'apache-arrow';
import type { DataType, RecordBatch, Schema } from 'apache-arrow';

// columnsOf はスキーマのフィールド名を列名の配列として返す。
export function columnsOf(schema: Schema): string[] {
  return schema.fields.map((f) => f.name);
}

// isEpochMillisecondsType は値がエポックミリ秒 (あるいはエポック日) で返される型かを返す。
export function isEpochMillisecondsType(type: DataType | undefined): boolean {
  switch (type?.typeId) {
    case Type.Date:
    case Type.DateDay:
    case Type.DateMillisecond:
    case Type.Timestamp:
    case Type.TimestampSecond:
    case Type.TimestampMillisecond:
    case Type.TimestampMicrosecond:
    case Type.TimestampNanosecond:
      return true;
    default:
      return false;
  }
}

// cellToString は Arrow の値 1 つを表示用の文字列にする。type は列の Arrow 型
// (省略時は値だけから判定する)。
export function cellToString(value: unknown, type?: DataType): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (isEpochMillisecondsType(type)) {
    const ms = typeof value === 'bigint' ? Number(value) : value;
    if (typeof ms === 'number') {
      const date = new Date(ms);
      if (!Number.isNaN(date.getTime())) return date.toISOString();
    }
  }
  if (typeof value === 'bigint') return value.toString();
  if (typeof value === 'number') return String(value);
  if (value instanceof Date) return value.toISOString();
  return String(value);
}

// batchToRows はレコードバッチを行の配列へ変換する。列の順序はスキーマの定義順で固定される。
export function batchToRows(batch: RecordBatch): string[][] {
  const types = batch.schema.fields.map((f) => f.type);
  const vectors = batch.data.children.map((_, i) => batch.getChildAt(i));
  const rows: string[][] = [];
  for (let r = 0; r < batch.numRows; r += 1) {
    rows.push(vectors.map((v, i) => cellToString(v?.get(r), types[i])));
  }
  return rows;
}

// QueryBatchReader は collectQueryResult が読むストリームの最小形。DuckDB Wasm の
// AsyncDuckDBConnection.send() が返す Arrow の AsyncRecordBatchStreamReader はこの形を
// 満たす。テストではこの形のスタブを渡す。
//
// Arrow の RecordBatchReader は、open() (または最初の next()) がストリーム先頭のスキーマの
// メッセージを読むまで schema が undefined のままである。send() は open() せずに返すため、
// schema を読む側が先に open() を呼ぶ (docs/issues/closed/0206)。
export interface QueryBatchReader {
  open(): Promise<unknown>;
  readonly schema: Schema | undefined;
  [Symbol.asyncIterator](): AsyncIterator<RecordBatch>;
}

// CollectedQueryResult は collectQueryResult の結果。truncated は maxRows で打ち切ったかを示す。
export interface CollectedQueryResult {
  columns: string[];
  rows: string[][];
  truncated: boolean;
}

// collectQueryResult はレコードバッチのストリームを maxRows 行まで読み、打ち切った場合は
// cancel を呼んでからクエリを終える。打ち切りの判定は 2 経路ある。
// - 読み込み中のバッチに maxRows を超える行がある (そのバッチの残りを捨てる)
// - maxRows ちょうどで読み終え、行のあるバッチがまだ存在する (先読みして確かめる)
// maxRows ちょうどで残りが無い場合は打ち切りではないので cancel を呼ばない。
export async function collectQueryResult(
  reader: QueryBatchReader,
  maxRows: number,
  cancel: () => Promise<void>,
): Promise<CollectedQueryResult> {
  // スキーマはストリームの先頭にあり、open() で読む。open() の前に schema を読むと undefined
  // で列名を取れない (docs/issues/closed/0206)。スキーマの無いストリームでは Arrow が open()
  // でリーダーを閉じるため、列も行も無い結果になる。
  await reader.open();
  const columns = reader.schema === undefined ? [] : columnsOf(reader.schema);
  const rows: string[][] = [];
  let truncated = false;
  const iterator = reader[Symbol.asyncIterator]();
  for (;;) {
    const next = await iterator.next();
    if (next.done) break;
    const batch = batchToRows(next.value);
    const remaining = maxRows - rows.length;
    if (batch.length > remaining) {
      // このバッチの中で上限に達した。残り行を捨てるので打ち切りが確定する。
      rows.push(...batch.slice(0, remaining));
      truncated = true;
      break;
    }
    rows.push(...batch);
    if (rows.length >= maxRows) {
      // 上限ちょうど。行が残っているかを先読みして打ち切りかどうかを決める。末尾に 0 行の
      // バッチが続くことがあるため、行のあるバッチが出るまで読み進める (0 行のバッチの
      // 存在だけで打ち切りにすると、ちょうど上限で終わるクエリに打ち切りの表示が出る)。
      // 0 行のバッチだけを無限に返すストリームではこのループは終わらないが、それは上の
      // 本体のループも同じで、DuckDB のレコードバッチのストリームでは起きない。
      for (;;) {
        const lookahead = await iterator.next();
        if (lookahead.done) break;
        if (lookahead.value.numRows > 0) {
          truncated = true;
          break;
        }
      }
      break;
    }
  }
  if (truncated) {
    // 残りを読まずにクエリ自体を止める (読み続けるとメモリを消費する)。
    await cancel();
  }
  return { columns, rows, truncated };
}
