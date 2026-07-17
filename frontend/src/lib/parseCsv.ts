// RFC 4180 準拠の CSV パーサ。ダブルクォートで囲まれたフィールド内のカンマ・改行・
// "" エスケープに対応する。素朴な split(',') はこれらを壊すため独自実装する。
// 戻り値は行ごとの文字列配列 (1 行目もヘッダとして特別扱いせずそのまま返す)。
export function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = '';
  let inQuotes = false;
  let i = 0;
  const len = text.length;

  const pushField = () => {
    row.push(field);
    field = '';
  };
  const pushRow = () => {
    pushField();
    rows.push(row);
    row = [];
  };

  while (i < len) {
    const ch = text[i];

    if (inQuotes) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          field += '"';
          i += 2;
          continue;
        }
        inQuotes = false;
        i++;
        continue;
      }
      field += ch;
      i++;
      continue;
    }

    if (ch === '"') {
      inQuotes = true;
      i++;
      continue;
    }
    if (ch === ',') {
      pushField();
      i++;
      continue;
    }
    if (ch === '\r') {
      // CRLF は \n 側でまとめて処理する。単独 CR も改行として扱う。
      if (text[i + 1] === '\n') {
        i++;
        continue;
      }
      pushRow();
      i++;
      continue;
    }
    if (ch === '\n') {
      pushRow();
      i++;
      continue;
    }
    field += ch;
    i++;
  }

  // 末尾に改行が無い場合の最終フィールド/行を回収する。
  // 完全な空文字列の入力は 0 行として扱う。
  if (field !== '' || row.length > 0) {
    pushRow();
  }

  return rows;
}
