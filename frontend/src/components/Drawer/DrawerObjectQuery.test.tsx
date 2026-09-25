// DrawerObjectQuery のコンポーネントテスト。OPFS / Web Worker / DuckDB Wasm は jsdom で
// 動かないため、エンジンと取り込み処理は props のインターフェースへモックを差し込む。
import { StrictMode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { act, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerObjectQuery } from './DrawerObjectQuery';
import { ObjectQueryIngestError } from '../../lib/objectQuery';
import type { ObjectQueryEngine, ObjectQueryResult } from '../../lib/objectQuery';
import type {
  ObjectIngestProgress,
  ObjectIngestRequest,
  ObjectIngestor,
} from '../../lib/opfsIngest';
import { ApiError } from '../../types/common';

const MAX_BYTES = 1 << 30;

function makeEngine(overrides: Partial<ObjectQueryEngine> = {}): ObjectQueryEngine {
  return {
    registerView: vi.fn(async () => {}),
    run: vi.fn(async (): Promise<ObjectQueryResult> => ({
      columns: ['name'],
      rows: [['alice']],
      truncated: false,
      elapsedMs: 12,
    })),
    cancelSent: vi.fn(async () => {}),
    dropView: vi.fn(async () => {}),
    dropFile: vi.fn(async () => {}),
    ...overrides,
  };
}

// pendingIngestor は ingest を保留したままにし、テストが任意のタイミングで完了・失敗させる。
function pendingIngestor() {
  let resolveIngest: () => void = () => {};
  let rejectIngest: (err: unknown) => void = () => {};
  const ingestor: ObjectIngestor = {
    ingest: vi.fn(
      (_request: ObjectIngestRequest, onProgress: (p: ObjectIngestProgress) => void) => {
        onProgress({ written: 1024, total: 4096 });
        return new Promise<void>((resolve, reject) => {
          resolveIngest = resolve;
          rejectIngest = reject;
        });
      },
    ),
    terminate: vi.fn(async () => {
      const err = new Error('aborted');
      err.name = 'AbortError';
      rejectIngest(err);
    }),
  };
  return {
    ingestor,
    resolveIngest: () => resolveIngest(),
    rejectIngest: (e: unknown) => rejectIngest(e),
  };
}

// doneIngestor は ingest が即座に完了するインジェスタを返す。
function doneIngestor() {
  const ingestor: ObjectIngestor = {
    ingest: vi.fn(
      async (_request: ObjectIngestRequest, onProgress: (p: ObjectIngestProgress) => void) => {
        onProgress({ written: 4096, total: 4096 });
      },
    ),
    terminate: vi.fn(async () => {}),
  };
  return ingestor;
}

// SSOExpiredBanner が useSSOLogin (react-query の mutation) を使うため、Provider を挟む。
function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function renderQuery(
  engine: ObjectQueryEngine,
  ingestor: ObjectIngestor,
  extra: { profile?: string; fileName?: string } = {},
) {
  const onClose = vi.fn();
  const createIngestor = vi.fn(() => ingestor);
  const view = renderWithQC(
    <DrawerObjectQuery
      fileName={extra.fileName ?? 'data.csv'}
      url="http://127.0.0.1:8089/api/aws/profiles/p/s3/b/objects/download?key=data.csv"
      maxBytes={MAX_BYTES}
      size={4096}
      profile={extra.profile}
      engine={engine}
      createIngestor={createIngestor}
      onClose={onClose}
    />,
  );
  return { ...view, onClose, createIngestor, ingestor };
}

describe('DrawerObjectQuery', () => {
  it('取り込み中の進捗を Content-Length 付きで表示する', async () => {
    const engine = makeEngine();
    const { ingestor, container } = renderQuery(engine, pendingIngestor().ingestor);

    await waitFor(() => {
      expect(container.textContent).toContain('取り込み中 1.0 KB / 4.0 KB (25%)');
    });
    expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    // 取り込みが終わるまで Query の実行には進まない
    expect(engine.run).not.toHaveBeenCalled();
  });

  it('取り込み完了後に obj ビューを作成して初期クエリの結果を表に表示する', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.resolveIngest();
    });

    await waitFor(() => {
      expect(container.textContent).toContain('alice');
    });
    // OPFS パスは登録名と SQL 中のパスで同じ文字列を使う
    const registerCall = (engine.registerView as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(registerCall[0]).toMatch(/^opfs:\/\/thief-query\/.+\.csv$/);
    expect(registerCall[1]).toBe(`read_csv('${registerCall[0]}')`);
    // csv は DuckDB の拡張を要しない
    expect(registerCall[2]).toEqual([]);
    expect(engine.run).toHaveBeenCalledWith('SELECT * FROM obj LIMIT 100');
    // 行数と実行時間を表の下に出す
    expect(container.textContent).toContain('1 行');
    expect(container.textContent).toContain('12 ms');
    // ヘッダには元オブジェクトのキーを出す
    expect(container.textContent).toContain('Query: data.csv');
  });

  it('json 形式では読み取り関数の拡張 (json) を registerView に渡す', async () => {
    const engine = makeEngine();
    const { container } = renderQuery(engine, doneIngestor(), { fileName: 'logs/data.json.gz' });

    await waitFor(() => {
      expect(container.textContent).toContain('alice');
    });
    const registerCall = (engine.registerView as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(registerCall[0]).toMatch(/^opfs:\/\/thief-query\/.+\.json\.gz$/);
    expect(registerCall[1]).toBe(`read_json_auto('${registerCall[0]}')`);
    expect(registerCall[2]).toEqual(['json']);
  });

  it('取り込みエラーが SSO 期限切れのときは SSOExpiredBanner を出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor, {
      profile: 'my-profile',
    });

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(new ApiError(401, 'SSO_TOKEN_EXPIRED', 'token expired'));
    });

    await waitFor(() => {
      expect(container.textContent).toContain('SSO セッションが期限切れです');
    });
    expect(container.textContent).toContain('取り込みに失敗しました');
    expect(container.textContent).not.toContain('token expired');
  });

  it('profile が無い場合は SSO 期限切れでも ErrorBanner に出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(new ApiError(401, 'SSO_TOKEN_EXPIRED', 'token expired'));
    });

    await waitFor(() => {
      expect(container.textContent).toContain('token expired');
    });
    expect(container.textContent).not.toContain('SSO セッションが期限切れです');
  });

  it('その他の取り込みエラーは ErrorBanner にメッセージを出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(new ApiError(500, 'INTERNAL_ERROR', 'download failed'));
    });

    await waitFor(() => {
      expect(container.textContent).toContain('download failed');
    });
    expect(container.textContent).toContain('取り込みに失敗しました');
    expect(engine.registerView).not.toHaveBeenCalled();
  });

  it('上限超過の取り込みエラーは上限付きの文言を出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(
        new ObjectQueryIngestError('tooLarge', 'object is larger than the limit'),
      );
    });

    await waitFor(() => {
      expect(container.textContent).toContain(
        'サイズ上限 (1.0 GiB) を超えているため取り込みを中止しました',
      );
    });
  });

  it('Worker が非対応を返したときはブラウザ非対応の文言を出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(
        new ObjectQueryIngestError(
          'unsupported',
          'FileSystemFileHandle.createSyncAccessHandle is not available in this browser',
        ),
      );
    });

    await waitFor(() => {
      expect(container.textContent).toContain('このブラウザは OPFS への書き込みに対応していません');
    });
    expect(engine.registerView).not.toHaveBeenCalled();
  });

  it('空き容量不足の取り込みエラーは容量不足の文言を出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(new ObjectQueryIngestError('quota', 'not enough storage'));
    });

    await waitFor(() => {
      expect(container.textContent).toContain(
        'ブラウザの空き容量が足りないため取り込みを開始できません',
      );
    });
  });

  it('OPFS のエラーは DuckDB 以外の取り込みエラーとしてメッセージを出す', async () => {
    const engine = makeEngine();
    const pending = pendingIngestor();
    const { ingestor, container } = renderQuery(engine, pending.ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      pending.rejectIngest(new ObjectQueryIngestError('opfs', 'failed to open sync access handle'));
    });

    await waitFor(() => {
      expect(container.textContent).toContain('failed to open sync access handle');
    });
    expect(container.textContent).toContain('取り込みに失敗しました');
  });

  it('Content-Length が無いときは書き込み量だけの進捗を表示する', async () => {
    const engine = makeEngine();
    const ingestor: ObjectIngestor = {
      ingest: vi.fn(
        (_request: ObjectIngestRequest, onProgress: (p: ObjectIngestProgress) => void) => {
          onProgress({ written: 2048, total: null });
          return new Promise<void>(() => {});
        },
      ),
      terminate: vi.fn(async () => {}),
    };
    const { container } = renderQuery(engine, ingestor);

    await waitFor(() => {
      expect(container.textContent).toContain('取り込み中 2.0 KB');
    });
    // total が無いので割合は出さない
    expect(container.textContent).not.toContain('%');
  });

  it('クエリエラーは見出しと DuckDB のメッセージを出す', async () => {
    const engine = makeEngine({
      run: vi.fn(async () => {
        throw new Error('Parser Error: syntax error at or near "SELEC"');
      }),
    });
    const { container } = renderQuery(engine, doneIngestor());

    await waitFor(() => {
      expect(container.textContent).toContain('クエリに失敗しました');
    });
    expect(container.textContent).toContain('Parser Error: syntax error');
  });

  it('10000 行で打ち切ったときは打ち切りの案内を出す', async () => {
    const rows = Array.from({ length: 10000 }, (_, i) => [`row-${i}`]);
    const engine = makeEngine({
      run: vi.fn(async () => ({
        columns: ['name'],
        rows,
        truncated: true,
        elapsedMs: 100,
      })),
    });
    const { container } = renderQuery(engine, doneIngestor());

    await waitFor(() => {
      expect(container.textContent).toContain('10000 行を超えたため打ち切りました');
    });
    expect(container.textContent).toContain('10000 行');
  });

  it('取り込み要求に URL と OPFS パスとロック名と上限とサイズを渡す', async () => {
    const engine = makeEngine();
    const { ingestor } = renderQuery(engine, pendingIngestor().ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });
    const request = (ingestor.ingest as ReturnType<typeof vi.fn>).mock
      .calls[0][0] as ObjectIngestRequest;
    expect(request.url).toBe(
      'http://127.0.0.1:8089/api/aws/profiles/p/s3/b/objects/download?key=data.csv',
    );
    expect(request.storageName).toMatch(/^thief-query\/.+\.csv$/);
    expect(request.maxBytes).toBe(MAX_BYTES);
    expect(request.size).toBe(4096);
    // ロック名と OPFS パスは同じファイル名を指す (別々に採番しない)。
    const fileName = request.storageName.slice('thief-query/'.length);
    expect(request.lockName).toBe(`thief-query:${fileName}`);
  });

  it('解放の途中で失敗しても、後続の段階と Worker の終了指示まで到達する', async () => {
    const engine = makeEngine({
      cancelSent: vi.fn(async () => {
        throw new Error('cancel failed');
      }),
      dropView: vi.fn(async () => {
        throw new Error('drop view failed');
      }),
    });
    const ingestor = doneIngestor();
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { unmount } = renderQuery(engine, ingestor);

    await waitFor(() => {
      expect(engine.run).toHaveBeenCalledTimes(1);
    });

    unmount();

    await waitFor(() => {
      expect(engine.dropFile).toHaveBeenCalledTimes(1);
      expect(ingestor.terminate).toHaveBeenCalledTimes(1);
    });
    expect(engine.cancelSent).toHaveBeenCalledTimes(1);
    expect(engine.dropView).toHaveBeenCalledTimes(1);
    // 失敗した段階は警告に留める。
    expect(warn).toHaveBeenCalledTimes(2);
    warn.mockRestore();
  });

  it('アンマウントすると cancelSent / DROP VIEW / dropFile と Worker の終了指示を呼ぶ', async () => {
    const engine = makeEngine();
    const ingestor = doneIngestor();
    const { unmount, container } = renderQuery(engine, ingestor);

    await waitFor(() => {
      expect(engine.run).toHaveBeenCalledTimes(1);
    });
    expect(container.textContent).toContain('alice');

    unmount();

    await waitFor(() => {
      expect(engine.cancelSent).toHaveBeenCalledTimes(1);
      expect(engine.dropView).toHaveBeenCalledTimes(1);
      expect(engine.dropFile).toHaveBeenCalledTimes(1);
      expect(ingestor.terminate).toHaveBeenCalledTimes(1);
    });
    const dropFileCall = (engine.dropFile as ReturnType<typeof vi.fn>).mock.calls[0];
    const dropViewCall = (engine.dropView as ReturnType<typeof vi.fn>).mock.calls[0];
    const registerCall = (engine.registerView as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(dropFileCall[0]).toBe(registerCall[0]);
    // dropView には登録と同じ OPFS パスを渡す (別の文字列だと解放が永久に一致しなくなる)。
    expect(dropViewCall[0]).toBe(registerCall[0]);
  });

  it('取り込み中にアンマウントしても Worker の終了指示まで到達し、エラーを表示しない', async () => {
    const engine = makeEngine();
    const { ingestor, unmount, container } = renderQuery(engine, pendingIngestor().ingestor);

    await waitFor(() => {
      expect(ingestor.ingest).toHaveBeenCalledTimes(1);
    });

    unmount();

    await waitFor(() => {
      expect(ingestor.terminate).toHaveBeenCalledTimes(1);
      expect(engine.cancelSent).toHaveBeenCalledTimes(1);
    });
    // 中断はエラーとして表示しない
    expect(container.textContent).not.toContain('取り込みに失敗しました');
  });

  it('StrictMode の二重実行でも取り込みは 1 回だけで、初期クエリの結果を表示する', async () => {
    const engine = makeEngine();
    const ingestor = doneIngestor();
    const onClose = vi.fn();
    const { container } = renderWithQC(
      <StrictMode>
        <DrawerObjectQuery
          fileName="data.csv"
          url="http://127.0.0.1:8089/download"
          maxBytes={MAX_BYTES}
          size={4096}
          engine={engine}
          createIngestor={() => ingestor}
          onClose={onClose}
        />
      </StrictMode>,
    );

    // setup → cleanup → setup の 2 回目だけが取り込みを開始する (二重ダウンロードしない)
    await waitFor(() => {
      expect(engine.registerView).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(container.textContent).toContain('alice');
    });
    expect(ingestor.ingest).toHaveBeenCalledTimes(1);
  });
});
