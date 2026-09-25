// OPFS への取り込み専用 Worker。メインスレッドから受け取ったダウンロード API の URL を
// fetch し、レスポンスの ReadableStream を読みながら OPFS のファイルへ
// createSyncAccessHandle() の write() で書く。
//
// このファイルはアプリのモジュール (i18n を含む) を import しない。ログとエラーメッセージは
// 英語のまま Worker 内で完結させ、表示用の文言はメインスレッドが kind から選ぶ。
// 型は opfsIngest.ts から import type で取り込む (実行時 import は残らない)。上限判定と
// エラー判定は opfsWriterLimits.ts の純関数を使う (同ファイルも同じ制約を満たし、判定だけを
// 単体テストで固定するために切り出してある)。
//
// 取り込みからアンマウントまでの間、Web Locks のロックを保持する。起動時の残骸削除
// (lib/opfs.ts) はロックを取得できたファイルだけを削除するため、他タブが使用中のファイルは
// 消されない。終了指示 (terminate) を受けたら、中断していた場合は fetch を AbortController で
// 止めてからファイルを削除し、ロックを解放する。
import type { OpfsWriterEvent, OpfsWriterRequest } from './opfsIngest';
import {
  exceedsMaxBytes,
  isFetchAbortError,
  isNotFoundError,
  isQuotaError,
  parseContentLength,
} from './opfsWriterLimits';

// lib.dom.d.ts には FileSystemFileHandle.createSyncAccessHandle の宣言が無く、その型は
// lib.webworker.d.ts にだけある。tsconfig に WebWorker の lib を足すと DOM の型と衝突する
// ため、Worker で必要な形だけを宣言する。
interface SyncAccessHandle {
  write(buffer: BufferSource, options?: { at?: number }): number;
  flush(): void;
  close(): void;
}

interface SyncAccessFileHandle extends FileSystemFileHandle {
  createSyncAccessHandle(): Promise<SyncAccessHandle>;
}

// PROGRESS_REPORT_BYTES は進捗を postMessage する間隔。チャンクごとに送ると 1 GiB の
// 取り込みで数千回の再描画が起きるため、書き込み量がある程度増えるまでまとめる。
const PROGRESS_REPORT_BYTES = 256 * 1024;

// ObjectIngestOutcome は OPFS への書き込み 1 回の結果。
type ObjectIngestOutcome =
  | { type: 'done'; written: number }
  | { type: 'aborted' }
  | { type: 'error'; event: Extract<OpfsWriterEvent, { type: 'error' }> };

let abortController: AbortController | null = null;
let releaseLock: (() => void) | null = null;
let terminated = false;

function post(event: OpfsWriterEvent): void {
  self.postMessage(event);
}

// errorMessage は例外から表示可能なメッセージを取り出す。
function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// readErrorBody は backend の ErrorResponse から code と message を取り出す。
async function readErrorBody(response: Response): Promise<{ code?: string; message: string }> {
  try {
    const body = (await response.json()) as { error?: string; code?: string; message?: string };
    return {
      code: typeof body.code === 'string' ? body.code : undefined,
      message:
        typeof body.error === 'string'
          ? body.error
          : typeof body.message === 'string'
            ? body.message
            : response.statusText,
    };
  } catch {
    return { message: response.statusText };
  }
}

// removeEntry は thief-query/ 配下のファイルを削除する。失敗しても取り込みの後始末は
// 続けるため、例外は投げずに警告に留める。
async function removeEntry(dirName: string, fileName: string): Promise<void> {
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle(dirName, { create: false });
    await dir.removeEntry(fileName);
  } catch (err) {
    if (isNotFoundError(err)) return;
    console.warn('failed to remove OPFS file', err);
  }
}

// writeToOpfs はファイルを作成してレスポンスを書き込む。ファイル名から形式と圧縮を DuckDB が
// 判定できるよう、OPFS 上のファイル名は元の拡張子 (圧縮の拡張子を含む) を保つ。
async function writeToOpfs(
  request: Extract<OpfsWriterRequest, { type: 'ingest' }>,
): Promise<ObjectIngestOutcome> {
  let access: SyncAccessHandle | null = null;
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle(request.dirName, { create: true });
    const fileHandle = (await dir.getFileHandle(request.fileName, {
      create: true,
    })) as SyncAccessFileHandle;
    access = await fileHandle.createSyncAccessHandle();

    const controller = new AbortController();
    abortController = controller;
    if (terminated) {
      // createSyncAccessHandle の待機中に終了指示を受けていた場合、この時点では
      // abortController がまだ無いため onmessage の abort() が届いていない。fetch を
      // 始めずに中断する (始めると 5 秒の強制終了まで転送が続く)。
      return { type: 'aborted' };
    }
    let response: Response;
    try {
      response = await fetch(request.url, { signal: controller.signal });
    } catch (err) {
      if (isFetchAbortError(err)) return { type: 'aborted' };
      return {
        type: 'error',
        event: { type: 'error', kind: 'network', message: errorMessage(err) },
      };
    }

    if (!response.ok) {
      const { code, message } = await readErrorBody(response);
      return {
        type: 'error',
        event: { type: 'error', kind: 'http', status: response.status, code, message },
      };
    }

    const total = parseContentLength(response.headers.get('Content-Length'));
    if (total !== null && exceedsMaxBytes(total, request.maxBytes)) {
      // Content-Length が上限を超える場合は読み始めずに中断する。
      return {
        type: 'error',
        event: {
          type: 'error',
          kind: 'tooLarge',
          message: `object is larger than the limit (${total} > ${request.maxBytes})`,
        },
      };
    }
    if (!response.body) {
      return {
        type: 'error',
        event: { type: 'error', kind: 'opfs', message: 'response body is not readable' },
      };
    }

    let written = 0;
    let reported = 0;
    const reader = response.body.getReader();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        if (!value || value.byteLength === 0) continue;
        access.write(value, { at: written });
        written += value.byteLength;
        if (exceedsMaxBytes(written, request.maxBytes)) {
          // 累積が上限を超えた場合は読み込みを中断する (ファイルは後始末で削除する)。
          controller.abort();
          return {
            type: 'error',
            event: {
              type: 'error',
              kind: 'tooLarge',
              message: `object is larger than the limit (${written} > ${request.maxBytes})`,
            },
          };
        }
        if (written - reported >= PROGRESS_REPORT_BYTES) {
          reported = written;
          post({ type: 'progress', written, total });
        }
      }
    } catch (err) {
      if (isFetchAbortError(err)) return { type: 'aborted' };
      if (isQuotaError(err)) {
        return {
          type: 'error',
          event: { type: 'error', kind: 'quota', message: errorMessage(err) },
        };
      }
      return { type: 'error', event: { type: 'error', kind: 'opfs', message: errorMessage(err) } };
    }
    post({ type: 'progress', written, total });
    access.flush();
    return { type: 'done', written };
  } catch (err) {
    if (isQuotaError(err)) {
      return { type: 'error', event: { type: 'error', kind: 'quota', message: errorMessage(err) } };
    }
    return { type: 'error', event: { type: 'error', kind: 'opfs', message: errorMessage(err) } };
  } finally {
    abortController = null;
    try {
      access?.flush();
    } catch {
      // flush の失敗は close で後始末する
    }
    try {
      access?.close();
    } catch {
      // 既に閉じている場合の close 失敗は無視する
    }
  }
}

// startIngest は取り込み 1 回の全体 (ロック取得 → 書き込み → 終了指示待ち → 後始末) を行う。
async function startIngest(request: Extract<OpfsWriterRequest, { type: 'ingest' }>): Promise<void> {
  // createSyncAccessHandle は Worker 専用 API のため、取り込みの開始時に Worker 側で確認する。
  if (!('createSyncAccessHandle' in FileSystemFileHandle.prototype)) {
    post({
      type: 'error',
      kind: 'unsupported',
      message: 'FileSystemFileHandle.createSyncAccessHandle is not available in this browser',
    });
    post({ type: 'closed' });
    self.close();
    return;
  }

  try {
    await navigator.locks.request(request.lockName, async () => {
      // ロックは OPFS のファイルを作成する前に取得する。作成後に取ると、書き込み開始直後の
      // ファイルが他タブの起動時の残骸削除に消されうる。
      const outcome = await writeToOpfs(request);
      if (outcome.type === 'done') {
        post({ type: 'done', written: outcome.written });
        if (!terminated) {
          // 完了後もロックとファイルを保持し、メインスレッドの終了指示を待つ
          // (クエリ中のファイルを他タブの起動時の削除から守る)。
          await new Promise<void>((resolve) => {
            releaseLock = resolve;
          });
          releaseLock = null;
        }
        return;
      }
      if (outcome.type === 'error') {
        post(outcome.event);
      } else {
        post({ type: 'aborted' });
      }
    });
  } catch (err) {
    post({ type: 'error', kind: 'opfs', message: errorMessage(err) });
  }
  // 完了後の終了指示・失敗・中断のいずれでもファイルを残さない。
  await removeEntry(request.dirName, request.fileName);
  post({ type: 'closed' });
  self.close();
}

self.onmessage = (event: MessageEvent<OpfsWriterRequest>) => {
  const request = event.data;
  if (request.type === 'ingest') {
    void startIngest(request);
    return;
  }
  // terminate: 実行中の fetch を中断し、完了後の終了指示待ちを解除する。ファイルの削除と
  // ロックの解放は startIngest の後始末が行う。
  terminated = true;
  abortController?.abort();
  abortController = null;
  releaseLock?.();
  releaseLock = null;
};
