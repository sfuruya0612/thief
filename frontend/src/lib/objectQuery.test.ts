import { describe, expect, it } from 'vitest';
import {
  clampObjectQuerySelection,
  formatObjectQueryLimit,
  isAbortError,
  ObjectQueryIngestError,
  objectQueryDisabledReason,
  objectQueryExtensionInitSql,
  objectQueryExtensions,
  objectQueryFileName,
  objectQueryFormat,
  objectQueryInstallSql,
  objectQueryLockName,
  objectQueryOpfsPath,
  objectQueryOverLimit,
  objectQueryQuotaExceeded,
  objectQuerySelectionDisabledReason,
  objectQueryStorageName,
  objectQueryViewSql,
  OBJECT_QUERY_EXTENSION_REPOSITORY,
  OBJECT_QUERY_INGEST_CONCURRENCY,
  OBJECT_QUERY_MAX_FILES,
  OBJECT_QUERY_MAX_ROWS,
} from './objectQuery';
import type { ObjectQueryTarget } from './objectQuery';

const READY_1GIB = { status: 'ready', maxBytes: 1 << 30 } as const;

describe('objectQueryFormat', () => {
  it('設計判断 5 の表の 11 形式を判定する', () => {
    const cases: [string, string, string][] = [
      ['data.csv', 'read_csv', '.csv'],
      ['data.csv.gz', 'read_csv', '.csv.gz'],
      ['data.tsv', 'read_csv', '.tsv'],
      ['data.tsv.gz', 'read_csv', '.tsv.gz'],
      ['data.json', 'read_json_auto', '.json'],
      ['data.json.gz', 'read_json_auto', '.json.gz'],
      ['data.jsonl', 'read_json_auto', '.jsonl'],
      ['data.jsonl.gz', 'read_json_auto', '.jsonl.gz'],
      ['data.ndjson', 'read_json_auto', '.ndjson'],
      ['data.ndjson.gz', 'read_json_auto', '.ndjson.gz'],
      ['data.parquet', 'read_parquet', '.parquet'],
    ];
    for (const [key, readFunction, extension] of cases) {
      const format = objectQueryFormat(key);
      expect(format?.readFunction, key).toBe(readFunction);
      expect(format?.extension, key).toBe(extension);
    }
  });

  it('大文字小文字を区別しない', () => {
    expect(objectQueryFormat('DATA.CSV')?.extension).toBe('.csv');
    expect(objectQueryFormat('data.Parquet')?.readFunction).toBe('read_parquet');
  });

  it('ディレクトリ名ではなくファイル名の拡張子で判定する', () => {
    expect(objectQueryFormat('logs.parquet/notes.txt')).toBeUndefined();
  });

  it('対象外の拡張子は undefined', () => {
    expect(objectQueryFormat('app.log')).toBeUndefined();
    expect(objectQueryFormat('archive.gz')).toBeUndefined();
    expect(objectQueryFormat('data.avro')).toBeUndefined();
    expect(objectQueryFormat('data.orc')).toBeUndefined();
    expect(objectQueryFormat('README')).toBeUndefined();
  });

  it('tsv だけ区切り文字を持つ', () => {
    expect(objectQueryFormat('data.tsv')?.delimiter).toBe('\\t');
    expect(objectQueryFormat('data.csv')?.delimiter).toBeUndefined();
  });
});

describe('objectQueryExtensions / objectQueryInstallSql', () => {
  it('json 系は json、parquet は parquet、csv / tsv は拡張を要しない', () => {
    for (const key of [
      'data.json',
      'data.json.gz',
      'data.jsonl',
      'data.jsonl.gz',
      'data.ndjson',
      'data.ndjson.gz',
    ]) {
      const format = objectQueryFormat(key);
      expect(format && objectQueryExtensions(format), key).toEqual(['json']);
    }
    const parquet = objectQueryFormat('data.parquet');
    expect(parquet && objectQueryExtensions(parquet)).toEqual(['parquet']);
    for (const key of ['data.csv', 'data.csv.gz', 'data.tsv', 'data.tsv.gz']) {
      const format = objectQueryFormat(key);
      expect(format && objectQueryExtensions(format), key).toEqual([]);
    }
  });

  it('INSTALL と LOAD を並べた SQL を組み立てる', () => {
    expect(objectQueryInstallSql('json')).toBe('INSTALL json; LOAD json;');
    expect(objectQueryInstallSql('parquet')).toBe('INSTALL parquet; LOAD parquet;');
  });
});

describe('objectQueryExtensionInitSql', () => {
  it('autoinstall / autoload を止め、拡張リポジトリを同一オリジンにする', () => {
    expect(objectQueryExtensionInitSql()).toEqual([
      'SET autoinstall_known_extensions = false',
      'SET autoload_known_extensions = false',
      `SET custom_extension_repository = '${OBJECT_QUERY_EXTENSION_REPOSITORY}'`,
    ]);
    expect(OBJECT_QUERY_EXTENSION_REPOSITORY).toBe('/assets/duckdb-extensions');
  });
});

describe('objectQueryViewSql', () => {
  it('複数の OPFS パスをリスト引数にした読み取り関数と object_key 列でビューを組み立てる', () => {
    const format = objectQueryFormat('data.parquet');
    const files = [
      {
        opfsPath: 'opfs://thief-query/u1.parquet',
        fileName: 'u1.parquet',
        key: 'out/part-00000.parquet',
      },
      {
        opfsPath: 'opfs://thief-query/u2.parquet',
        fileName: 'u2.parquet',
        key: 'out/part-00001.parquet',
      },
    ];
    expect(format && objectQueryViewSql(files, format)).toBe(
      'CREATE OR REPLACE VIEW obj AS SELECT * REPLACE (CASE ' +
        "WHEN object_key LIKE '%u1.parquet' THEN 'out/part-00000.parquet' " +
        "WHEN object_key LIKE '%u2.parquet' THEN 'out/part-00001.parquet' " +
        "ELSE object_key END AS object_key) FROM read_parquet(['opfs://thief-query/u1.parquet', 'opfs://thief-query/u2.parquet'], union_by_name = true, filename = 'object_key')",
    );
  });

  it('1 件でもリストの形にし、由来のキーを CASE 式で写す', () => {
    const format = objectQueryFormat('data.csv');
    const files = [{ opfsPath: 'opfs://thief-query/u1.csv', fileName: 'u1.csv', key: 'data.csv' }];
    expect(format && objectQueryViewSql(files, format)).toBe(
      'CREATE OR REPLACE VIEW obj AS SELECT * REPLACE (CASE ' +
        "WHEN object_key LIKE '%u1.csv' THEN 'data.csv' " +
        "ELSE object_key END AS object_key) FROM read_csv(['opfs://thief-query/u1.csv'], union_by_name = true, filename = 'object_key')",
    );
  });

  it('tsv では delim が付く', () => {
    const format = objectQueryFormat('data.tsv');
    const files = [
      { opfsPath: 'opfs://thief-query/u1.tsv', fileName: 'u1.tsv', key: 'data.tsv' },
      { opfsPath: 'opfs://thief-query/u2.tsv.gz', fileName: 'u2.tsv.gz', key: 'data.tsv.gz' },
    ];
    expect(format && objectQueryViewSql(files, format)).toBe(
      'CREATE OR REPLACE VIEW obj AS SELECT * REPLACE (CASE ' +
        "WHEN object_key LIKE '%u1.tsv' THEN 'data.tsv' " +
        "WHEN object_key LIKE '%u2.tsv.gz' THEN 'data.tsv.gz' " +
        "ELSE object_key END AS object_key) FROM read_csv(['opfs://thief-query/u1.tsv', 'opfs://thief-query/u2.tsv.gz'], union_by_name = true, filename = 'object_key', delim='\\t')",
    );
  });

  it('キーの単一引用符を二重化する', () => {
    const format = objectQueryFormat('data.csv');
    const files = [{ opfsPath: 'opfs://thief-query/u1.csv', fileName: 'u1.csv', key: "it's.csv" }];
    expect(format && objectQueryViewSql(files, format)).toContain("THEN 'it''s.csv'");
  });

  it('files が空なら組み立てずに例外を投げる', () => {
    const format = objectQueryFormat('data.csv');
    expect(() => format && objectQueryViewSql([], format)).toThrow(
      'objectQueryViewSql: files must not be empty',
    );
  });
});

describe('objectQueryFileName / storage / opfs path / lock name', () => {
  const format = objectQueryFormat('data.csv.gz');

  it('一致した拡張子を保ったファイル名を組み立てる', () => {
    expect(format && objectQueryFileName('abc', format)).toBe('abc.csv.gz');
  });

  it('OPFS の相対パスと DuckDB に登録するパスは同じファイル名を共有する', () => {
    const fileName = format ? objectQueryFileName('abc', format) : '';
    expect(objectQueryStorageName(fileName)).toBe('thief-query/abc.csv.gz');
    expect(objectQueryOpfsPath(fileName)).toBe('opfs://thief-query/abc.csv.gz');
  });

  it('ロック名はファイル名から再構成できる', () => {
    expect(objectQueryLockName('abc.csv.gz')).toBe('thief-query:abc.csv.gz');
  });
});

describe('objectQueryOverLimit', () => {
  it('上限ちょうどは許容する', () => {
    expect(objectQueryOverLimit(1024, 1024)).toBe(false);
    expect(objectQueryOverLimit(1025, 1024)).toBe(true);
  });
});

describe('objectQueryQuotaExceeded', () => {
  it('残り容量がサイズ未満なら true', () => {
    expect(objectQueryQuotaExceeded(1000, 500, 500)).toBe(false);
    expect(objectQueryQuotaExceeded(1000, 500, 501)).toBe(true);
  });
});

describe('formatObjectQueryLimit', () => {
  it('1024 進の単位で丸める', () => {
    expect(formatObjectQueryLimit(1 << 30)).toBe('1.0 GiB');
    expect(formatObjectQueryLimit(8 << 20)).toBe('8.0 MiB');
    expect(formatObjectQueryLimit(512)).toBe('512 B');
  });
});

describe('objectQueryDisabledReason', () => {
  it('対象形式・上限以下・設定取得済み・ブラウザ対応なら押せる', () => {
    expect(objectQueryDisabledReason('data.csv', 100, READY_1GIB, true)).toBe('');
  });

  it('ブラウザ非対応の理由を最優先で返す', () => {
    expect(objectQueryDisabledReason('data.csv', 100, READY_1GIB, false)).toBe(
      'このブラウザはオブジェクトの SQL 検索に対応していません',
    );
  });

  it('設定の取得失敗と取得中の理由を返す', () => {
    expect(objectQueryDisabledReason('data.csv', 100, { status: 'error' }, true)).toBe(
      '設定を取得できないため実行できません',
    );
    expect(objectQueryDisabledReason('data.csv', 100, { status: 'loading' }, true)).toBe(
      '設定を取得中です',
    );
  });

  it('対象外の拡張子の理由を返す', () => {
    expect(objectQueryDisabledReason('app.log', 100, READY_1GIB, true)).toBe(
      'SQL 検索の対象外の形式です',
    );
  });

  it('上限超過の理由を上限付きで返す', () => {
    expect(objectQueryDisabledReason('data.csv', (1 << 30) + 1, READY_1GIB, true)).toBe(
      'サイズ上限 (1.0 GiB) を超えるオブジェクトは SQL 検索できません',
    );
    expect(objectQueryDisabledReason('data.csv', 1 << 30, READY_1GIB, true)).toBe('');
  });
});

describe('objectQuerySelectionDisabledReason', () => {
  const target = (key: string, size = 100): ObjectQueryTarget => ({
    key,
    url: `http://127.0.0.1:8089/download?key=${key}`,
    size,
  });

  it('対象形式がそろっていれば押せる', () => {
    expect(objectQuerySelectionDisabledReason([target('data.csv')], READY_1GIB, true)).toBe('');
    expect(
      objectQuerySelectionDisabledReason(
        [target('data.csv'), target('data.csv.gz'), target('other.csv')],
        READY_1GIB,
        true,
      ),
    ).toBe('');
  });

  it('ブラウザ非対応 → 設定の取得失敗 → 取得中 → 0 件 の順に理由を返す', () => {
    expect(objectQuerySelectionDisabledReason([target('data.csv')], READY_1GIB, false)).toBe(
      'このブラウザはオブジェクトの SQL 検索に対応していません',
    );
    expect(
      objectQuerySelectionDisabledReason([target('data.csv')], { status: 'error' }, true),
    ).toBe('設定を取得できないため実行できません');
    expect(
      objectQuerySelectionDisabledReason([target('data.csv')], { status: 'loading' }, true),
    ).toBe('設定を取得中です');
    expect(objectQuerySelectionDisabledReason([], READY_1GIB, true)).toBe(
      'オブジェクトを選択してください',
    );
  });

  it('readFunction または delimiter が食い違う組み合わせは形式の混在として拒否する', () => {
    expect(
      objectQuerySelectionDisabledReason(
        [target('data.csv'), target('data.tsv')],
        READY_1GIB,
        true,
      ),
    ).toBe('形式の異なるオブジェクトはまとめて検索できません');
    expect(
      objectQuerySelectionDisabledReason(
        [target('data.parquet'), target('data.parquet')],
        READY_1GIB,
        true,
      ),
    ).toBe('');
    expect(
      objectQuerySelectionDisabledReason(
        [target('data.csv'), target('data.parquet')],
        READY_1GIB,
        true,
      ),
    ).toBe('形式の異なるオブジェクトはまとめて検索できません');
    expect(
      objectQuerySelectionDisabledReason(
        [target('data.tsv'), target('data.tsv.gz')],
        READY_1GIB,
        true,
      ),
    ).toBe('');
  });

  it('件数が OBJECT_QUERY_MAX_FILES を超えると拒否する', () => {
    const targets = Array.from({ length: OBJECT_QUERY_MAX_FILES }, (_, i) =>
      target(`data-${i}.csv`),
    );
    expect(objectQuerySelectionDisabledReason(targets, READY_1GIB, true)).toBe('');
    expect(
      objectQuerySelectionDisabledReason([...targets, target('extra.csv')], READY_1GIB, true),
    ).toBe('一度に選択できるオブジェクトは 50 件までです');
  });

  it('合計サイズが上限を超えると拒否し、ちょうどは許す', () => {
    const half = (1 << 30) / 2;
    expect(
      objectQuerySelectionDisabledReason(
        [target('a.csv', half), target('b.csv', half)],
        READY_1GIB,
        true,
      ),
    ).toBe('');
    expect(
      objectQuerySelectionDisabledReason(
        [target('a.csv', half), target('b.csv', half + 1)],
        READY_1GIB,
        true,
      ),
    ).toBe('選択したオブジェクトの合計サイズが上限 (1.0 GiB) を超えています');
  });
});

describe('clampObjectQuerySelection', () => {
  it('集合の挿入順の先頭 max 件だけを残す', () => {
    expect([...clampObjectQuerySelection(new Set(['a', 'b', 'c']), 2)]).toEqual(['a', 'b']);
  });

  it('max 件以下なら同じ要素の集合を返す', () => {
    expect([...clampObjectQuerySelection(new Set(['a', 'b']), 2)]).toEqual(['a', 'b']);
    expect([...clampObjectQuerySelection(new Set(), 1)]).toEqual([]);
  });
});

describe('OBJECT_QUERY_MAX_FILES / OBJECT_QUERY_INGEST_CONCURRENCY', () => {
  it('件数の上限は 50、並列度は 4', () => {
    expect(OBJECT_QUERY_MAX_FILES).toBe(50);
    expect(OBJECT_QUERY_INGEST_CONCURRENCY).toBe(4);
  });
});

describe('ObjectQueryIngestError / isAbortError', () => {
  it('kind を保持する', () => {
    const err = new ObjectQueryIngestError('quota', 'not enough storage');
    expect(err.kind).toBe('quota');
    expect(err.message).toBe('not enough storage');
    expect(err.name).toBe('ObjectQueryIngestError');
  });

  it('AbortError だけを中断として判定する', () => {
    const abort = new Error('aborted');
    abort.name = 'AbortError';
    expect(isAbortError(abort)).toBe(true);
    expect(isAbortError(new Error('other'))).toBe(false);
    expect(isAbortError('aborted')).toBe(false);
  });
});

describe('OBJECT_QUERY_MAX_ROWS', () => {
  it('打ち切りの行数は 10000 固定', () => {
    expect(OBJECT_QUERY_MAX_ROWS).toBe(10000);
  });
});
