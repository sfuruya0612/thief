import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerS3Objects } from './DrawerS3Objects';

// 取り込み処理 (Worker + OPFS) は jsdom で動かない。実物を使うとアンマウント時の終了指示が
// 応答を待って 5 秒のタイムアウトに入り、OPFS の残骸削除も警告を出すため、ingestor を
// モックする。取り込み後の表示は DrawerObjectQuery.test.tsx が検証する。
vi.mock('../../lib/opfsIngest', () => ({
  createOpfsIngestor: () => ({
    ingest: vi.fn(() => new Promise<void>(() => {})),
    terminate: vi.fn(async () => {}),
  }),
}));

// テスト間で QueryClient を独立させるためのラッパー
function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    json: async () => body,
  } as Response;
}

const CONFIG_RESPONSE = { object_query_max_bytes: 1 << 30 };

// fetch のモックを URL で分岐させる。/api/config (オブジェクト SQL 検索の設定) には固定の
// 応答を返し、それ以外は handler が返した応答を返す。handler が undefined を返した URL への
// 呼び出しはテストの誤りとして例外にする。
function mockFetchByUrl(handler: (url: string, init?: RequestInit) => Response | undefined) {
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.includes('/api/config')) {
      return jsonResponse(CONFIG_RESPONSE);
    }
    const res = handler(url, init);
    if (!res) throw new Error(`unexpected fetch: ${url}`);
    return res;
  });
  globalThis.fetch = mock as unknown as typeof fetch;
  return mock;
}

// apiCalls は /api/config 以外の呼び出し (/api/... のリソース系) だけを呼び出し順に返す。
function apiCalls(fetchMock: ReturnType<typeof vi.fn>): unknown[][] {
  return fetchMock.mock.calls.filter((call) => !String(call[0]).includes('/api/config'));
}

// stubObjectQueryBrowserSupport は Query ボタンの活性判定が要求するブラウザ機能
// (OPFS と Web Locks) を jsdom に与える。
function stubObjectQueryBrowserSupport() {
  Object.defineProperty(globalThis.navigator, 'storage', {
    value: { getDirectory: () => Promise.resolve() },
    configurable: true,
  });
  Object.defineProperty(globalThis.navigator, 'locks', {
    value: { request: () => Promise.resolve() },
    configurable: true,
  });
}

function objectQueryButtons(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll('button')).filter(
    (b) => b.textContent === 'Query',
  ) as HTMLButtonElement[];
}

describe('DrawerS3Objects', () => {
  const originalFetch = globalThis.fetch;
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');
  const originalLocks = Object.getOwnPropertyDescriptor(globalThis.navigator, 'locks');

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    // テストごとに差し替えたブラウザ機能のスタブを元に戻す
    if (originalStorage) {
      Object.defineProperty(globalThis.navigator, 'storage', originalStorage);
    } else {
      Reflect.deleteProperty(globalThis.navigator, 'storage');
    }
    if (originalLocks) {
      Object.defineProperty(globalThis.navigator, 'locks', originalLocks);
    } else {
      Reflect.deleteProperty(globalThis.navigator, 'locks');
    }
    vi.restoreAllMocks();
  });

  it('S3 オブジェクト一覧を key/size/storage_class 付きで表示する', async () => {
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          {
            key: 'path/to/file.txt',
            size: 2048,
            last_modified: '2026-07-08T00:00:00Z',
            storage_class: 'STANDARD',
            etag: 'abc',
          },
        ],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('path/to/file.txt');
    });
    expect(container.textContent).toContain('STANDARD');
    // formatBytes(2048) は "2.0 KB"
    expect(container.textContent).toContain('2.0 KB');
    // ダウンロードリンクが download 属性付きで生成されている
    const dl = container.querySelector('a[download]') as HTMLAnchorElement | null;
    expect(dl).not.toBeNull();
    expect(dl!.getAttribute('href')).toContain('/objects/download');
    expect(dl!.getAttribute('href')).toContain('key=path%2Fto%2Ffile.txt');
    // 行アクションは Preview / Query / Download の 3 つ
    const actionCells = container.querySelectorAll('table.dt tbody tr td:last-child button');
    expect(Array.from(actionCells).map((b) => b.textContent)).toEqual(['Preview', 'Query']);
  });

  it('取得件数が上限に達した場合は打ち切りの通知を表示する', async () => {
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: true,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('a.txt');
    });
    expect(container.textContent).toContain('上限');
  });

  it('ファイル選択とアップロードボタン押下で multipart POST を送る', async () => {
    const fetchMock = mockFetchByUrl((url) =>
      url.includes('/objects/upload')
        ? jsonResponse({}, 204)
        : jsonResponse({ objects: [], truncated: false }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    // 一覧の初回 GET が発火するのを待つ
    await waitFor(() => {
      expect(apiCalls(fetchMock)).toHaveLength(1);
    });

    const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
    fireEvent.change(fileInput, { target: { files: [file] } });

    // button 要素で絞る (Download リンクも "Download" テキストを持つため)
    const uploadBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'Upload',
    ) as HTMLButtonElement;
    expect(uploadBtn.disabled).toBe(false);
    fireEvent.click(uploadBtn);

    await waitFor(() => {
      expect(apiCalls(fetchMock).length).toBeGreaterThanOrEqual(2);
    });
    // 2 回目 (config を除く) の呼び出し = アップロード
    const uploadCall = apiCalls(fetchMock)[1];
    expect(uploadCall[0]).toContain('/objects/upload');
    expect(uploadCall[0]).toContain('key=hello.txt');
    const init = uploadCall[1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
  });

  it('prefix 入力だけでは再取得されず、検索ボタン押下でサーバへ prefix 付きの再取得を要求する', async () => {
    const fetchMock = mockFetchByUrl((url) =>
      jsonResponse({
        objects: url.includes('prefix=logs')
          ? [
              {
                key: 'logs/a.txt',
                size: 1,
                last_modified: '',
                storage_class: 'STANDARD',
                etag: '1',
              },
            ]
          : [
              {
                key: 'logs/a.txt',
                size: 1,
                last_modified: '',
                storage_class: 'STANDARD',
                etag: '1',
              },
              {
                key: 'other/b.txt',
                size: 1,
                last_modified: '',
                storage_class: 'STANDARD',
                etag: '2',
              },
            ],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('logs/a.txt');
    });
    expect(container.textContent).toContain('other/b.txt');
    expect(apiCalls(fetchMock)).toHaveLength(1);

    const prefixInput = container.querySelector(
      'input[placeholder="prefix (folder/subfolder)…"]',
    ) as HTMLInputElement;
    fireEvent.change(prefixInput, { target: { value: '/logs' } });

    // 入力だけでは一覧 GET は再実行されない
    expect(apiCalls(fetchMock)).toHaveLength(1);

    const searchBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === '検索',
    ) as HTMLButtonElement;
    fireEvent.click(searchBtn);

    // 検索ボタン押下でサーバへ prefix 付きの再取得が走る
    await waitFor(() => {
      expect(apiCalls(fetchMock)).toHaveLength(2);
    });
    const searchCall = apiCalls(fetchMock)[1];
    expect(searchCall[0]).toContain('prefix=logs');

    await waitFor(() => {
      expect(container.textContent).not.toContain('other/b.txt');
    });
    expect(container.textContent).toContain('logs/a.txt');
  });

  it('prefix を入力してアップロードすると key に prefix が付与される', async () => {
    const fetchMock = mockFetchByUrl((url) =>
      url.includes('/objects/upload')
        ? jsonResponse({}, 204)
        : jsonResponse({ objects: [], truncated: false }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(apiCalls(fetchMock)).toHaveLength(1);
    });

    const prefixInput = container.querySelector(
      'input[placeholder="prefix (folder/subfolder)…"]',
    ) as HTMLInputElement;
    fireEvent.change(prefixInput, { target: { value: '/logs/' } });

    // prefix 入力だけでは一覧 GET は再実行されない
    expect(apiCalls(fetchMock)).toHaveLength(1);

    const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
    fireEvent.change(fileInput, { target: { files: [file] } });

    const uploadBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'Upload',
    ) as HTMLButtonElement;
    fireEvent.click(uploadBtn);

    await waitFor(() => {
      expect(apiCalls(fetchMock).length).toBeGreaterThanOrEqual(2);
    });
    const uploadCall = apiCalls(fetchMock)[1];
    expect(uploadCall[0]).toContain('/objects/upload');
    expect(uploadCall[0]).toContain('key=logs%2Fhello.txt');
  });

  it('バイナリ拡張子や 5 MB 以上のオブジェクトは Preview ボタンを無効化し行をグレーアウトする', async () => {
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'ok.log', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          { key: 'image.png', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '2' },
          {
            key: 'huge.txt',
            size: 5 * 1024 * 1024,
            last_modified: '',
            storage_class: 'STANDARD',
            etag: '3',
          },
        ],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('ok.log');
    });

    const previewButtons = Array.from(container.querySelectorAll('button')).filter(
      (b) => b.textContent === 'Preview',
    );
    expect(previewButtons).toHaveLength(3);
    const [okBtn, pngBtn, hugeBtn] = previewButtons;
    // テキスト拡張子 (.log) はプレビュー可能
    expect(okBtn.disabled).toBe(false);
    expect(pngBtn.disabled).toBe(true);
    expect(pngBtn.title).toBe('バイナリファイルはプレビューできません');
    expect(hugeBtn.disabled).toBe(true);
    expect(hugeBtn.title).toBe('5 MB 以上のオブジェクトはプレビューできません');

    // プレビュー不可の行だけ preview-ineligible クラスでグレーアウトされる
    const dataRows = Array.from(container.querySelectorAll('table.dt tbody tr'));
    const rowFor = (text: string) =>
      dataRows.find((tr) => tr.textContent?.includes(text)) as HTMLTableRowElement;
    expect(rowFor('ok.log').classList.contains('preview-ineligible')).toBe(false);
    expect(rowFor('image.png').classList.contains('preview-ineligible')).toBe(true);
    expect(rowFor('huge.txt').classList.contains('preview-ineligible')).toBe(true);
  });

  it('Preview ボタンをクリックするとプレビュー API を呼び中身を表示する', async () => {
    const fetchMock = mockFetchByUrl((url) =>
      url.includes('/objects/preview')
        ? jsonResponse({ content: 'hello preview', content_type: 'text/plain', size: 13 })
        : jsonResponse({
            objects: [
              {
                key: 'notes.txt',
                size: 100,
                last_modified: '',
                storage_class: 'STANDARD',
                etag: '1',
              },
            ],
            truncated: false,
          }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('notes.txt');
    });

    const previewBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'Preview',
    ) as HTMLButtonElement;
    fireEvent.click(previewBtn);

    await waitFor(() => {
      expect(container.textContent).toContain('hello preview');
    });

    const previewCall = apiCalls(fetchMock)[1];
    expect(previewCall[0]).toContain('/objects/preview');
    expect(previewCall[0]).toContain('key=notes.txt');
  });

  it('プレビューを編集して保存すると同じキーへ upload し確認ダイアログを挟む', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      if (url.includes('/objects/preview')) {
        return jsonResponse({ content: 'hello preview', content_type: 'text/plain', size: 13 });
      }
      if (url.includes('/objects/upload')) {
        return jsonResponse({}, 204);
      }
      return jsonResponse({
        objects: [
          {
            key: 'notes.txt',
            size: 100,
            last_modified: '',
            storage_class: 'STANDARD',
            etag: '1',
          },
        ],
        truncated: false,
      });
    });

    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('notes.txt');
    });

    const previewBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'Preview',
    ) as HTMLButtonElement;
    fireEvent.click(previewBtn);

    await waitFor(() => {
      expect(container.textContent).toContain('hello preview');
    });

    const editBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === '編集',
    ) as HTMLButtonElement;
    fireEvent.click(editBtn);

    const textarea = container.querySelector(
      'textarea.object-edit-textarea',
    ) as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: 'edited content' } });

    const saveBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === '保存',
    ) as HTMLButtonElement;
    fireEvent.click(saveBtn);

    expect(confirmSpy).toHaveBeenCalled();

    await waitFor(() => {
      expect(apiCalls(fetchMock).length).toBeGreaterThanOrEqual(3);
    });
    const saveCall = apiCalls(fetchMock)[2];
    expect(saveCall[0]).toContain('/objects/upload');
    expect(saveCall[0]).toContain('key=notes.txt');
    const init = saveCall[1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);

    confirmSpy.mockRestore();
  });

  it('Query ボタンは対象形式・上限以下・設定取得済み・ブラウザ対応の行だけ押せる', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          {
            key: 'data.parquet',
            size: 100,
            last_modified: '',
            storage_class: 'STANDARD',
            etag: '2',
          },
          { key: 'app.log', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '3' },
          {
            key: 'huge.csv',
            size: (1 << 30) + 1,
            last_modified: '',
            storage_class: 'STANDARD',
            etag: '4',
          },
        ],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('data.parquet');
    });
    // 設定の取得完了を待つ (取得完了までは Query が無効)
    await waitFor(() => {
      expect(objectQueryButtons(container)[0].disabled).toBe(false);
    });

    const [csvBtn, parquetBtn, logBtn, hugeBtn] = objectQueryButtons(container);
    expect(csvBtn.disabled).toBe(false);
    expect(csvBtn.title).toBe('オブジェクトに SQL を実行');
    expect(parquetBtn.disabled).toBe(false);
    expect(logBtn.disabled).toBe(true);
    expect(logBtn.title).toBe('SQL 検索の対象外の形式です');
    expect(hugeBtn.disabled).toBe(true);
    expect(hugeBtn.title).toBe('サイズ上限 (1.0 GiB) を超えるオブジェクトは SQL 検索できません');
  });

  it('Query ボタンを押すと一覧から SQL 検索のパネルへ切り替わる', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(objectQueryButtons(container)[0]?.disabled).toBe(false);
    });
    fireEvent.click(objectQueryButtons(container)[0]);

    await waitFor(() => {
      expect(container.textContent).toContain('Query: data.csv');
    });
    // 一覧は表示されなくなり、取り込みの進捗表示に切り替わる
    expect(container.textContent).not.toContain('Objects (1)');
    expect(container.textContent).toContain('取り込み中');
  });

  it('SQL 検索のパネルを開いたまま region を変えると一覧に戻る (解放されないままにしない)', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      }),
    );

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { container, rerender } = render(
      <QueryClientProvider client={qc}>
        <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(objectQueryButtons(container)[0]?.disabled).toBe(false);
    });
    fireEvent.click(objectQueryButtons(container)[0]);
    await waitFor(() => {
      expect(container.textContent).toContain('Query: data.csv');
    });

    // バケットの一覧は region を問わず同じなので、選択も Drawer も残ったまま region だけが
    // 変わる。パネルが開いたままだと OPFS のファイルとロックが解放されないため作り直す。
    rerender(
      <QueryClientProvider client={qc}>
        <DrawerS3Objects profile="test" region="us-east-1" bucket="my-bucket" />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Objects (1)');
    });
    expect(container.textContent).not.toContain('Query: data.csv');
  });

  it('設定の取得中は Query ボタンを無効化し理由を title に出す', async () => {
    stubObjectQueryBrowserSupport();
    const fetchMock = mockFetchByUrl((url) => {
      if (url.includes('/api/config')) return undefined;
      return jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      });
    });
    // /api/config だけ応答させない (取得中のまま)
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/config')) {
        return new Promise<Response>(() => {});
      }
      return jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('data.csv');
    });
    const [queryBtn] = objectQueryButtons(container);
    expect(queryBtn.disabled).toBe(true);
    expect(queryBtn.title).toBe('設定を取得中です');
  });

  it('設定の取得に失敗したときは Query ボタンを無効化し理由を title に出す', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl((url, init) => {
      void init;
      if (url.includes('/api/config')) {
        return undefined;
      }
      return jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      });
    });
    // /api/config が 404 を返す (旧版の backend) ケース
    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/config')) {
        return jsonResponse({ error: 'not found', code: 'NOT_FOUND' }, 404);
      }
      return jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('data.csv');
    });
    await waitFor(() => {
      expect(objectQueryButtons(container)[0].disabled).toBe(true);
    });
    expect(objectQueryButtons(container)[0].title).toBe('設定を取得できないため実行できません');
  });

  it('ブラウザが非対応のときは Query ボタンを無効化し理由を title に出す', async () => {
    // navigator.storage / navigator.locks をスタブしない (jsdom の既定 = 非対応)
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('data.csv');
    });
    await waitFor(() => {
      expect(objectQueryButtons(container)[0].disabled).toBe(true);
    });
    expect(objectQueryButtons(container)[0].title).toBe(
      'このブラウザはオブジェクトの SQL 検索に対応していません',
    );
  });
});
