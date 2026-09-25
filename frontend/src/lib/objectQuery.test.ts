import { describe, expect, it } from 'vitest';
import {
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
  objectQueryReadSql,
  objectQueryStorageName,
  OBJECT_QUERY_EXTENSION_REPOSITORY,
  OBJECT_QUERY_MAX_ROWS,
} from './objectQuery';

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

describe('objectQueryReadSql', () => {
  it('拡張子から決まる読み取り関数を OPFS パスに適用する', () => {
    const csv = objectQueryFormat('data.csv');
    expect(csv && objectQueryReadSql('opfs://thief-query/x.csv', csv)).toBe(
      "read_csv('opfs://thief-query/x.csv')",
    );
    const tsv = objectQueryFormat('data.tsv.gz');
    expect(tsv && objectQueryReadSql('opfs://thief-query/x.tsv.gz', tsv)).toBe(
      "read_csv('opfs://thief-query/x.tsv.gz', delim='\\t')",
    );
    const jsonl = objectQueryFormat('data.jsonl');
    expect(jsonl && objectQueryReadSql('opfs://thief-query/x.jsonl', jsonl)).toBe(
      "read_json_auto('opfs://thief-query/x.jsonl')",
    );
    const parquet = objectQueryFormat('data.parquet');
    expect(parquet && objectQueryReadSql('opfs://thief-query/x.parquet', parquet)).toBe(
      "read_parquet('opfs://thief-query/x.parquet')",
    );
  });

  it('パス中の単一引用符をエスケープする', () => {
    const format = objectQueryFormat('data.csv');
    expect(format && objectQueryReadSql("opfs://thief-query/it's.csv", format)).toBe(
      "read_csv('opfs://thief-query/it''s.csv')",
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
