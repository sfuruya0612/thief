import { describe, expect, it } from 'vitest';
import { isSuppressedViteWarning, withSuppressedWarnings } from './viteLogger';

// 抑止対象の警告は Vite が picocolors で色付け (ANSI エスケープ) して渡す。テストでは色付けの
// 有無を切り替え、包含判定がどちらでも成立することを確認する。
const ANSI_YELLOW = '\u001b[33m';
const ANSI_RESET = '\u001b[39m';

type WarningParts = {
  workerPath: string;
  arrowPath: string;
  wording: string;
};

const DEFAULT_PARTS: WarningParts = {
  workerPath: '/work/node_modules/@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js',
  arrowPath: '/work/node_modules/@duckdb/apache-arrow/util/util/buffer.ts',
  wording: 'points to a source file outside its package',
};

// warningMessage は背景に引用した形の警告を組み立てる。
function warningMessage(parts: Partial<WarningParts> = {}, color = false): string {
  const { workerPath, arrowPath, wording } = { ...DEFAULT_PARTS, ...parts };
  const msg = `Sourcemap for "${workerPath}" ${wording}: "${arrowPath}"`;
  return color ? `${ANSI_YELLOW}${msg}${ANSI_RESET}` : msg;
}

type LogCall = { method: 'warn' | 'warnOnce'; msg: string; options: unknown };

// createRecordingLogger は Vite の createLogger の代わりに、warn / warnOnce の呼び出しを
// 記録するロガーを返す。
function createRecordingLogger() {
  const calls: LogCall[] = [];
  const logger = {
    warn(msg: string, options?: unknown): void {
      calls.push({ method: 'warn', msg, options });
    },
    warnOnce(msg: string, options?: unknown): void {
      calls.push({ method: 'warnOnce', msg, options });
    },
  };
  return { logger, calls };
}

describe('isSuppressedViteWarning', () => {
  it('背景に引用した形の警告なら true', () => {
    expect(isSuppressedViteWarning(warningMessage())).toBe(true);
  });

  it('ANSI エスケープで色付けされていても true', () => {
    expect(isSuppressedViteWarning(warningMessage({}, true))).toBe(true);
  });

  it('別の worker ファイル (mvp) なら false', () => {
    expect(
      isSuppressedViteWarning(
        warningMessage({
          workerPath: '/work/node_modules/@duckdb/duckdb-wasm/dist/duckdb-browser-mvp.worker.js',
        }),
      ),
    ).toBe(false);
  });

  it('@duckdb/apache-arrow 以外のパスを指すなら false', () => {
    expect(
      isSuppressedViteWarning(
        warningMessage({
          arrowPath: '/work/node_modules/@duckdb/duckdb-wasm/src/duckdb-browser-eh.worker.ts',
        }),
      ),
    ).toBe(false);
  });

  it('文言が違えば false', () => {
    expect(
      isSuppressedViteWarning(warningMessage({ wording: 'points to missing source files' })),
    ).toBe(false);
  });

  it('無関係な警告なら false', () => {
    expect(isSuppressedViteWarning('Sourcemap for "/work/src/main.ts" is missing')).toBe(false);
  });
});

describe('withSuppressedWarnings', () => {
  it('抑止対象のメッセージでは元の warn / warnOnce を呼ばない', () => {
    const { logger, calls } = createRecordingLogger();
    const wrapped = withSuppressedWarnings(logger, isSuppressedViteWarning);
    wrapped.warn(warningMessage());
    wrapped.warnOnce(warningMessage({}, true));
    expect(calls).toEqual([]);
  });

  it('抑止対象でないメッセージでは同じ引数で元の warn / warnOnce を呼ぶ', () => {
    const { logger, calls } = createRecordingLogger();
    const wrapped = withSuppressedWarnings(logger, isSuppressedViteWarning);
    const options = { timestamp: true };
    wrapped.warn('plain warning', options);
    wrapped.warnOnce('once warning', options);
    expect(calls).toEqual([
      { method: 'warn', msg: 'plain warning', options },
      { method: 'warnOnce', msg: 'once warning', options },
    ]);
  });

  it('渡したオブジェクトと同じオブジェクトを返す', () => {
    const { logger } = createRecordingLogger();
    expect(withSuppressedWarnings(logger, isSuppressedViteWarning)).toBe(logger);
  });
});
