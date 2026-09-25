// OPFS への取り込み (メインスレッド側)。専用 Worker (opfsWriter.worker.ts) を起動し、
// ダウンロード API の URL から OPFS への書き込みを依頼する。Worker とは postMessage で
// 進捗・完了・エラーを受け渡す。
//
// HTTP エラーは backend の ErrorResponse から ApiError に組み直し、ネットワーク到達不能は
// client.ts と同じ ApiError(0, 'network_error') に正規化する (client.ts の doFetch は
// export されておらず Worker から使えないため、Worker 側で同じ形に正規化して受け取る)。
import { ApiError } from '../types/common';
import { ObjectQueryIngestError, objectQueryQuotaExceeded } from './objectQuery';
import { removeObjectQueryFile } from './opfs';

// ObjectIngestProgress は取り込みの進捗。total は Content-Length が無い場合 null。
export interface ObjectIngestProgress {
  written: number;
  total: number | null;
}

// ObjectIngestRequest は取り込みの依頼。
export interface ObjectIngestRequest {
  url: string;
  // storageName は OPFS ルートからの相対パス (例: "thief-query/<uuid>.csv")。
  storageName: string;
  // lockName は Web Locks のロック名 (例: "thief-query:<uuid>.csv")。
  lockName: string;
  maxBytes: number;
  // size は一覧が持つオブジェクトのサイズ。取り込み前のブラウザ空き容量チェックに使う。
  size: number;
}

// ObjectIngestor は OPFS への取り込み処理。DrawerObjectQuery はこのインターフェース越しに
// 取り込みを行い、テストではモックを差し込む。
export interface ObjectIngestor {
  // ingest は完了まで待ち、失敗は reject する。進捗のたびに onProgress を呼ぶ。
  ingest(
    request: ObjectIngestRequest,
    onProgress: (progress: ObjectIngestProgress) => void,
  ): Promise<void>;
  // terminate は実行中の取り込みを中断し、OPFS のファイルを削除してロックを解放する。
  // 未起動・二重呼び出しでも安全。
  terminate(): Promise<void>;
}

// OpfsWriterErrorKind は Worker が返すエラーの種別。http と network は ApiError に、
// それ以外は ObjectQueryIngestError に組み直す。
export type OpfsWriterErrorKind =
  'http' | 'network' | 'unsupported' | 'tooLarge' | 'quota' | 'opfs';

// OpfsWriterRequest はメインスレッドから Worker へ送るメッセージ。
export type OpfsWriterRequest =
  | {
      type: 'ingest';
      url: string;
      dirName: string;
      fileName: string;
      lockName: string;
      maxBytes: number;
    }
  | { type: 'terminate' };

// OpfsWriterEvent は Worker からメインスレッドへ送るメッセージ。closed はロックを解放して
// ファイルの後始末まで終えたことを示す (terminate の完了待ちに使う)。
export type OpfsWriterEvent =
  | { type: 'progress'; written: number; total: number | null }
  | { type: 'done'; written: number }
  | {
      type: 'error';
      kind: OpfsWriterErrorKind;
      status?: number;
      code?: string;
      message: string;
    }
  | { type: 'aborted' }
  | { type: 'closed' };

// TERMINATE_TIMEOUT_MS は Worker の終了応答 (closed) を待つ上限。応答が無い場合は Worker を
// 強制終了し、メインスレッドからファイル削除を試みる (Worker がクラッシュした場合の保険)。
const TERMINATE_TIMEOUT_MS = 5000;

// createOpfsIngestor は OPFS 取り込み用の Worker を 1 つ持つインジェスタを生成する。
export function createOpfsIngestor(): ObjectIngestor {
  return new OpfsIngestor();
}

// toIngestError は Worker のエラーイベントを表示用のエラーへ変換する。
function toIngestError(event: Extract<OpfsWriterEvent, { type: 'error' }>): unknown {
  if (event.kind === 'http') {
    return new ApiError(event.status ?? 0, event.code, event.message);
  }
  if (event.kind === 'network') {
    return new ApiError(0, 'network_error', event.message);
  }
  return new ObjectQueryIngestError(event.kind, event.message);
}

// createAbortError はアンマウント由来の中断を表すエラーを返す。表示しないための目印として
// name を AbortError にする (objectQuery.ts の isAbortError が判定する)。
function createAbortError(): Error {
  const err = new Error('object query ingestion aborted');
  err.name = 'AbortError';
  return err;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

class OpfsIngestor implements ObjectIngestor {
  private worker: Worker | null = null;
  private fileName: string | null = null;
  private terminated = false;
  private closed = false;
  private closedPromise: Promise<void> = Promise.resolve();
  private resolveClosed: (() => void) | null = null;
  private settle: { resolve: () => void; reject: (err: unknown) => void } | null = null;
  private onProgress: ((progress: ObjectIngestProgress) => void) | null = null;

  async ingest(
    request: ObjectIngestRequest,
    onProgress: (progress: ObjectIngestProgress) => void,
  ): Promise<void> {
    if (this.terminated) throw createAbortError();
    // 取り込み前の空き容量チェック。足りなければ Worker を起動しない (ダウンロードを始めない)。
    await this.checkQuota(request.size);
    // 空き容量の確認は待機を伴うため、その間にアンマウントされていることがある。Worker を
    // 起動すると誰も終了指示を送らないまま OPFS のファイルとロックを保持し続けるので、
    // 起動前にもう一度確認して中断する。
    if (this.terminated) throw createAbortError();

    const slash = request.storageName.lastIndexOf('/');
    if (slash <= 0 || slash === request.storageName.length - 1) {
      throw new ObjectQueryIngestError('opfs', `invalid storage name: ${request.storageName}`);
    }
    this.fileName = request.storageName.slice(slash + 1);
    this.onProgress = onProgress;
    this.closedPromise = new Promise((resolve) => {
      this.resolveClosed = resolve;
    });
    const worker = new Worker(new URL('./opfsWriter.worker.ts', import.meta.url), {
      type: 'module',
    });
    this.worker = worker;
    worker.onmessage = (event: MessageEvent<OpfsWriterEvent>) => this.handle(event.data);
    worker.onerror = (event: ErrorEvent) => this.handleWorkerError(event);

    const done = new Promise<void>((resolve, reject) => {
      this.settle = { resolve, reject };
    });
    const message: OpfsWriterRequest = {
      type: 'ingest',
      url: request.url,
      dirName: request.storageName.slice(0, slash),
      fileName: this.fileName,
      lockName: request.lockName,
      maxBytes: request.maxBytes,
    };
    worker.postMessage(message);
    return done;
  }

  async terminate(): Promise<void> {
    // Worker の起動前でもフラグは必ず立てる (ingest が起動前の待機中でも中断できるようにする)。
    this.terminated = true;
    const worker = this.worker;
    if (!worker) return;
    if (!this.closed) {
      const message: OpfsWriterRequest = { type: 'terminate' };
      worker.postMessage(message);
      await Promise.race([this.closedPromise, delay(TERMINATE_TIMEOUT_MS)]);
    }
    worker.terminate();
    this.worker = null;
    if (!this.closed && this.fileName) {
      // 応答が無いまま強制終了した場合の保険。Worker の終了でロックは解放されており、
      // ファイルはメインスレッドから削除できる。
      await removeObjectQueryFile(this.fileName);
    }
    // 取り込みが未 settle なら中断として返す (await している呼び出し元を解放する)。
    this.settleError(createAbortError());
  }

  private async checkQuota(size: number): Promise<void> {
    let estimate: StorageEstimate;
    try {
      estimate = await navigator.storage.estimate();
    } catch (err) {
      // 空き容量が取得できない場合はチェックを飛ばす (書き込み時の QuotaExceededError で検知する)。
      console.warn('failed to estimate storage quota', err);
      return;
    }
    if (estimate.quota === undefined || estimate.usage === undefined) {
      // 値が入らない環境がある。空き容量 0 とみなすと取り込めなくなるので、estimate が
      // 失敗した場合と同じくチェックを飛ばす。
      console.warn('storage estimate has no quota or usage');
      return;
    }
    const { quota, usage } = estimate;
    if (objectQueryQuotaExceeded(quota, usage, size)) {
      throw new ObjectQueryIngestError(
        'quota',
        `not enough storage: quota=${quota} usage=${usage} size=${size}`,
      );
    }
  }

  private handle(event: OpfsWriterEvent): void {
    switch (event.type) {
      case 'progress':
        this.onProgress?.(event);
        return;
      case 'done':
        this.settleOk();
        return;
      case 'aborted':
        this.settleError(createAbortError());
        return;
      case 'error':
        this.settleError(toIngestError(event));
        return;
      case 'closed':
        this.closed = true;
        this.resolveClosed?.();
        this.resolveClosed = null;
        return;
    }
  }

  private handleWorkerError(event: ErrorEvent): void {
    this.closed = true;
    this.resolveClosed?.();
    this.resolveClosed = null;
    this.settleError(new ObjectQueryIngestError('opfs', event.message));
  }

  private settleOk(): void {
    const settle = this.settle;
    this.settle = null;
    settle?.resolve();
  }

  private settleError(err: unknown): void {
    const settle = this.settle;
    this.settle = null;
    settle?.reject(err);
  }
}
