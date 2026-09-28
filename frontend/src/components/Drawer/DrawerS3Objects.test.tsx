import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerS3Objects } from './DrawerS3Objects';
import type { ObjectIngestRequest } from '../../lib/opfsIngest';

// ingestCalls はモックした ingestor が受け取った取り込み要求 (複数選択で開くファイルの検証)。
const ingestCalls = vi.hoisted(() => ({ calls: [] as ObjectIngestRequest[] }));

// 取り込み処理 (Worker + OPFS) は jsdom で動かない。実物を使うとアンマウント時の終了指示が
// 応答を待って 5 秒のタイムアウトに入り、OPFS の残骸削除も警告を出すため、ingestor を
// モックする。取り込み後の表示は DrawerObjectQuery.test.tsx が検証する。
vi.mock('../../lib/opfsIngest', () => ({
  createOpfsIngestor: () => ({
    ingest: vi.fn((request: ObjectIngestRequest) => {
      ingestCalls.calls.push(request);
      return new Promise<void>(() => {});
    }),
    terminate: vi.fn(async () => {}),
  }),
  checkObjectQueryQuota: vi.fn(async () => {}),
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

// dataRows は DataTable の本体行 (ヘッダと列フィルタ行を除く) を返す。
function dataRows(container: HTMLElement): HTMLTableRowElement[] {
  return Array.from(container.querySelectorAll('table.dt tbody tr')) as HTMLTableRowElement[];
}

// nameCells は本体行の名前の列 (チェックボックスの次のセル) の表示文字列を行順に返す。
function nameCells(container: HTMLElement): string[] {
  return dataRows(container).map((tr) => tr.querySelectorAll('td')[1]?.textContent ?? '');
}

function prefixInputOf(container: HTMLElement): HTMLInputElement {
  return container.querySelector(
    'input[placeholder="prefix (folder/subfolder)…"]',
  ) as HTMLInputElement;
}

function buttonByText(container: HTMLElement, text: string): HTMLButtonElement {
  const btn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent === text);
  if (!btn) throw new Error(`button not found: ${text}`);
  return btn as HTMLButtonElement;
}

// folderButton は名前の列のフォルダ行のボタン (表示名は相対名) を返す。
function folderButton(container: HTMLElement, name: string): HTMLButtonElement {
  const btn = Array.from(container.querySelectorAll('button.object-folder-link')).find(
    (b) => b.textContent === name,
  );
  if (!btn) throw new Error(`folder button not found: ${name}`);
  return btn as HTMLButtonElement;
}

// breadcrumbButton はパンくずのボタン (ルートは翻訳文言、各階層はフォルダ名) を返す。
function breadcrumbButton(container: HTMLElement, name: string): HTMLButtonElement {
  const btn = Array.from(container.querySelectorAll('.object-breadcrumb button')).find(
    (b) => b.textContent === name,
  );
  if (!btn) throw new Error(`breadcrumb button not found: ${name}`);
  return btn as HTMLButtonElement;
}

// modeButton は表示モードのトグルのボタンを返す。
function modeButton(container: HTMLElement, label: string): HTMLButtonElement {
  const btn = Array.from(container.querySelectorAll('.seg button')).find(
    (b) => b.textContent === label,
  );
  if (!btn) throw new Error(`mode button not found: ${label}`);
  return btn as HTMLButtonElement;
}

// rowFor は名前の列に text を含む本体行を返す。
function rowFor(container: HTMLElement, text: string): HTMLTableRowElement {
  const row = dataRows(container).find((tr) => tr.textContent?.includes(text));
  if (!row) throw new Error(`row not found: ${text}`);
  return row;
}

// checkboxOf は行のチェックボックスを返す (チェックボックスが無い行は null)。
function checkboxOf(row: HTMLTableRowElement): HTMLInputElement | null {
  return row.querySelector('input.cb');
}

// headerCheckbox はヘッダのチェックボックスを返す。
function headerCheckbox(container: HTMLElement): HTMLInputElement {
  return container.querySelector('thead input.cb') as HTMLInputElement;
}

// selectableRowNames はチェックボックスを持つ本体行の名前を表示順に返す。
function selectableRowNames(container: HTMLElement): string[] {
  return dataRows(container)
    .filter((tr) => checkboxOf(tr) !== null)
    .map((tr) => tr.querySelectorAll('td')[1]?.textContent ?? '');
}

// querySelectedButton は「選択した N 件に Query」ボタンを返す。
function querySelectedButton(container: HTMLElement): HTMLButtonElement {
  const btn = Array.from(container.querySelectorAll('button')).find((b) =>
    (b.textContent ?? '').startsWith('選択した'),
  );
  if (!btn) throw new Error('query selected button not found');
  return btn as HTMLButtonElement;
}

// s3Object は一覧の応答 1 件を組み立てる。
function s3Object(key: string, size = 100) {
  return { key, size, last_modified: '', storage_class: 'STANDARD', etag: key };
}

describe('DrawerS3Objects', () => {
  const originalFetch = globalThis.fetch;
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');
  const originalLocks = Object.getOwnPropertyDescriptor(globalThis.navigator, 'locks');

  beforeEach(() => {
    globalThis.fetch = vi.fn();
    ingestCalls.calls.length = 0;
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
        prefixes: [],
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
        prefixes: [],
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
        : jsonResponse({ objects: [], prefixes: [], truncated: false }),
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
        prefixes: [],
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
        : jsonResponse({ objects: [], prefixes: [], truncated: false }),
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
        prefixes: [],
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
            prefixes: [],
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
        prefixes: [],
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
        prefixes: [],
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
        prefixes: [],
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
    // 行ごとの Query はその行 1 件だけを取り込む
    await waitFor(() => {
      expect(ingestCalls.calls).toHaveLength(1);
    });
    expect(ingestCalls.calls[0].url).toContain('key=data.csv');
    expect(ingestCalls.calls[0].size).toBe(100);
  });

  it('Query 可能なオブジェクト行にだけチェックボックスが出る', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          s3Object('data.csv'),
          s3Object('data.parquet'),
          s3Object('app.log'),
          s3Object('huge.csv', (1 << 30) + 1),
        ],
        prefixes: ['logs/'],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('data.parquet');
    });
    await waitFor(() => {
      expect(objectQueryButtons(container)[0]?.disabled).toBe(false);
    });

    // フォルダ行と Query 不可の行 (対象外の形式、上限超過) にはチェックボックスが無い
    expect(checkboxOf(rowFor(container, 'logs/'))).toBeNull();
    expect(checkboxOf(rowFor(container, 'data.csv'))).not.toBeNull();
    expect(checkboxOf(rowFor(container, 'data.parquet'))).not.toBeNull();
    expect(checkboxOf(rowFor(container, 'app.log'))).toBeNull();
    expect(checkboxOf(rowFor(container, 'huge.csv'))).toBeNull();
    expect(selectableRowNames(container)).toEqual(['data.csv', 'data.parquet']);
  });

  it('ヘッダのチェックボックスで表示中の Query 可能な行が全選択 / 全解除される', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [s3Object('a.csv', 10), s3Object('b.csv', 20), s3Object('app.log')],
        prefixes: [],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('b.csv');
    });
    await waitFor(() => {
      expect(querySelectedButton(container).disabled).toBe(true);
    });
    expect(querySelectedButton(container).textContent).toBe('選択した 0 件に Query');
    expect(querySelectedButton(container).title).toBe('オブジェクトを選択してください');
    expect(container.textContent).toContain('0 件選択中');

    fireEvent.click(headerCheckbox(container));

    expect(checkboxOf(rowFor(container, 'a.csv'))?.checked).toBe(true);
    expect(checkboxOf(rowFor(container, 'b.csv'))?.checked).toBe(true);
    // Query 不可の行は選ばれない
    expect(checkboxOf(rowFor(container, 'app.log'))).toBeNull();
    expect(headerCheckbox(container).checked).toBe(true);
    expect(container.textContent).toContain('2 件選択中');
    expect(querySelectedButton(container).disabled).toBe(false);
    expect(querySelectedButton(container).textContent).toBe('選択した 2 件に Query');

    fireEvent.click(headerCheckbox(container));

    expect(checkboxOf(rowFor(container, 'a.csv'))?.checked).toBe(false);
    expect(checkboxOf(rowFor(container, 'b.csv'))?.checked).toBe(false);
    expect(headerCheckbox(container).checked).toBe(false);
    expect(container.textContent).toContain('0 件選択中');
    expect(querySelectedButton(container).disabled).toBe(true);
  });

  it('Query 可能な行が 51 件以上のときは表示順の先頭 50 件で打ち切り、51 件目のチェックでは変わらない', async () => {
    stubObjectQueryBrowserSupport();
    const objects = Array.from({ length: 51 }, (_, i) =>
      s3Object(`file-${String(i).padStart(2, '0')}.csv`),
    );
    mockFetchByUrl(() => jsonResponse({ objects, prefixes: [], truncated: false }));

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('file-50.csv');
    });
    expect(selectableRowNames(container)).toHaveLength(51);

    fireEvent.click(headerCheckbox(container));

    // 表示順の先頭 50 件だけが選ばれる
    expect(container.textContent).toContain('50 件選択中 (上限 50 件)');
    const selectedRows = dataRows(container).filter((tr) => checkboxOf(tr)?.checked);
    expect(selectedRows).toHaveLength(50);
    expect(checkboxOf(rowFor(container, 'file-00.csv'))?.checked).toBe(true);
    expect(checkboxOf(rowFor(container, 'file-49.csv'))?.checked).toBe(true);
    expect(checkboxOf(rowFor(container, 'file-50.csv'))?.checked).toBe(false);
    // 一部だけ選択なのでヘッダは indeterminate になる
    expect(headerCheckbox(container).checked).toBe(false);
    expect(headerCheckbox(container).indeterminate).toBe(true);

    // 51 件目の行をチェックしても、打ち切りの先頭 50 件のまま変わらない
    fireEvent.click(checkboxOf(rowFor(container, 'file-50.csv')) as HTMLInputElement);

    expect(container.textContent).toContain('50 件選択中 (上限 50 件)');
    expect(checkboxOf(rowFor(container, 'file-50.csv'))?.checked).toBe(false);
    expect(checkboxOf(rowFor(container, 'file-00.csv'))?.checked).toBe(true);
    expect(checkboxOf(rowFor(container, 'file-49.csv'))?.checked).toBe(true);

    // indeterminate からのヘッダのクリックは全解除になる
    fireEvent.click(headerCheckbox(container));

    expect(container.textContent).toContain('0 件選択中');
    expect(dataRows(container).filter((tr) => checkboxOf(tr)?.checked)).toHaveLength(0);
  });

  it('「選択した N 件に Query」ボタンは 0 件・形式の混在・合計サイズの超過で無効になる', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          s3Object('data.csv'),
          s3Object('data.parquet'),
          s3Object('big-a.csv', 600 * 1024 * 1024),
          s3Object('big-b.csv', 600 * 1024 * 1024),
        ],
        prefixes: [],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('big-b.csv');
    });
    await waitFor(() => {
      expect(objectQueryButtons(container)[0]?.disabled).toBe(false);
    });

    // 0 件
    expect(querySelectedButton(container).disabled).toBe(true);
    expect(querySelectedButton(container).title).toBe('オブジェクトを選択してください');

    // 形式の混在 (csv と parquet)
    fireEvent.click(checkboxOf(rowFor(container, 'data.csv')) as HTMLInputElement);
    fireEvent.click(checkboxOf(rowFor(container, 'data.parquet')) as HTMLInputElement);
    expect(container.textContent).toContain('2 件選択中');
    expect(querySelectedButton(container).disabled).toBe(true);
    expect(querySelectedButton(container).title).toBe(
      '形式の異なるオブジェクトはまとめて検索できません',
    );

    // 同じ形式 (csv) でも合計サイズが上限 (1 GiB) を超えると無効
    fireEvent.click(checkboxOf(rowFor(container, 'data.parquet')) as HTMLInputElement);
    fireEvent.click(checkboxOf(rowFor(container, 'big-a.csv')) as HTMLInputElement);
    fireEvent.click(checkboxOf(rowFor(container, 'big-b.csv')) as HTMLInputElement);
    expect(container.textContent).toContain('3 件選択中');
    expect(querySelectedButton(container).disabled).toBe(true);
    expect(querySelectedButton(container).title).toBe(
      '選択したオブジェクトの合計サイズが上限 (1.0 GiB) を超えています',
    );
  });

  it('「選択した N 件に Query」ボタンを押すと選んだ行の files で SQL 検索が開く', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [s3Object('a.csv', 10), s3Object('b.csv', 20)],
        prefixes: [],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('b.csv');
    });
    await waitFor(() => {
      expect(objectQueryButtons(container)[0]?.disabled).toBe(false);
    });

    fireEvent.click(checkboxOf(rowFor(container, 'a.csv')) as HTMLInputElement);
    fireEvent.click(checkboxOf(rowFor(container, 'b.csv')) as HTMLInputElement);
    fireEvent.click(querySelectedButton(container));

    await waitFor(() => {
      expect(container.textContent).toContain('Query: 2 件のオブジェクト');
    });
    expect(container.textContent).toContain('取り込み中');
    await waitFor(() => {
      expect(ingestCalls.calls).toHaveLength(2);
    });
    expect(ingestCalls.calls.map((request) => request.url)).toEqual([
      expect.stringContaining('key=a.csv'),
      expect.stringContaining('key=b.csv'),
    ]);
    expect(ingestCalls.calls.map((request) => request.size)).toEqual([10, 20]);
  });

  it('prefix の確定、フォルダの移動、モードの切り替えで選択が空になる', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl((url) => {
      const u = new URL(url);
      const flat = u.searchParams.get('delimiter') !== '/';
      return jsonResponse({
        objects: [s3Object('root.csv')],
        prefixes: flat ? [] : ['logs/'],
        truncated: false,
      });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('root.csv');
    });
    await waitFor(() => {
      expect(checkboxOf(rowFor(container, 'root.csv'))).not.toBeNull();
    });

    // prefix の確定
    fireEvent.click(checkboxOf(rowFor(container, 'root.csv')) as HTMLInputElement);
    expect(container.textContent).toContain('1 件選択中');
    fireEvent.change(prefixInputOf(container), { target: { value: 'log' } });
    fireEvent.click(buttonByText(container, '検索'));
    expect(container.textContent).toContain('0 件選択中');

    // フォルダの移動
    await waitFor(() => {
      expect(checkboxOf(rowFor(container, 'root.csv'))).not.toBeNull();
    });
    fireEvent.click(checkboxOf(rowFor(container, 'root.csv')) as HTMLInputElement);
    expect(container.textContent).toContain('1 件選択中');
    fireEvent.click(folderButton(container, 'logs/'));
    expect(container.textContent).toContain('0 件選択中');

    // モードの切り替え
    await waitFor(() => {
      expect(checkboxOf(rowFor(container, 'root.csv'))).not.toBeNull();
    });
    fireEvent.click(checkboxOf(rowFor(container, 'root.csv')) as HTMLInputElement);
    expect(container.textContent).toContain('1 件選択中');
    fireEvent.click(modeButton(container, 'フラット'));
    expect(container.textContent).toContain('0 件選択中');
  });

  it('SQL 検索のパネルを開いたまま region を変えると一覧に戻る (解放されないままにしない)', async () => {
    stubObjectQueryBrowserSupport();
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'data.csv', size: 100, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        prefixes: [],
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
        prefixes: [],
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
        prefixes: [],
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
        prefixes: [],
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
        prefixes: [],
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
        prefixes: [],
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

  it('初期表示はバケットのルートを階層モードで取得し、フォルダ行を名前の列だけで描く', async () => {
    const fetchMock = mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          {
            key: 'readme.txt',
            size: 10,
            last_modified: '2026-07-08T00:00:00Z',
            storage_class: 'STANDARD',
            etag: '1',
          },
        ],
        prefixes: ['logs/', 'reports/'],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('readme.txt');
    });

    // 初回はバケットのルート (prefix なし) を階層モード (delimiter=/) で取得する
    const firstCall = new URL(String(apiCalls(fetchMock)[0][0]));
    expect(firstCall.searchParams.get('delimiter')).toBe('/');
    expect(firstCall.searchParams.get('prefix')).toBe('');

    // 見出しの件数はオブジェクトのみで、フォルダは含めない
    expect(container.textContent).toContain('Objects (1)');

    // ソートしていない初期表示はフォルダ行が先頭に並ぶ
    expect(nameCells(container)).toEqual(['logs/', 'reports/', 'readme.txt']);

    const folderRow = dataRows(container)[0];
    const cells = Array.from(folderRow.querySelectorAll('td'));
    // 名前の列はフォルダのアイコン付きのボタンで、title に完全な prefix を出す
    const folderBtn = cells[1].querySelector('button');
    expect(folderBtn).not.toBeNull();
    expect(folderBtn!.getAttribute('title')).toBe('logs/ を開く');
    // 名前以外の列と Actions 列は空
    for (const cell of cells.slice(2)) {
      expect(cell.textContent).toBe('');
      expect(cell.querySelector('button')).toBeNull();
      expect(cell.querySelector('a')).toBeNull();
    }
    // フォルダ行はプレビュー不可のグレーアウトにしない
    expect(folderRow.classList.contains('preview-ineligible')).toBe(false);

    // オブジェクト行は Actions 列 (Preview / Query / Download) を保つ
    const objectRow = dataRows(container)[2];
    const objectCells = Array.from(objectRow.querySelectorAll('td'));
    expect(Array.from(objectCells[5].querySelectorAll('button')).map((b) => b.textContent)).toEqual(
      ['Preview', 'Query'],
    );
    expect(objectCells[5].querySelector('a[download]')).not.toBeNull();
  });

  it('フォルダ行のクリックで潜り、パンくずで戻る。どちらも検索欄と確定済みの入力が空になる', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'logs/') {
        return jsonResponse({
          objects: [
            { key: 'logs/a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          ],
          prefixes: ['logs/2024/'],
          truncated: false,
        });
      }
      return jsonResponse({
        objects: [
          { key: 'readme.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        prefixes: ['logs/'],
        truncated: false,
      });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('readme.txt');
    });

    // 検索欄に入力して確定してからフォルダへ潜る。確定済みの入力が遷移で消えなければ、
    // 潜る先の prefix が "logs/lo" になり、ルートの応答が返って一覧が変わらない
    fireEvent.change(prefixInputOf(container), { target: { value: 'lo' } });
    fireEvent.click(buttonByText(container, '検索'));
    // 検索の応答が描かれる (読み込み中は一覧が空になる) のを待ってからフォルダへ潜る
    await waitFor(() => {
      expect(apiCalls(fetchMock)).toHaveLength(2);
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    expect(new URL(String(apiCalls(fetchMock)[1][0])).searchParams.get('prefix')).toBe('lo');
    fireEvent.click(folderButton(container, 'logs/'));

    await waitFor(() => {
      expect(nameCells(container)).toEqual(['2024/', 'a.txt']);
    });
    // 階層モードのオブジェクト行は今いるフォルダを除いた相対名で表示する
    expect(container.textContent).not.toContain('logs/a.txt');
    // 潜る先は今いるフォルダだけで取得する (確定済みの入力は空になっている)
    const drillCall = new URL(String(apiCalls(fetchMock)[2][0]));
    expect(drillCall.searchParams.get('prefix')).toBe('logs/');
    expect(drillCall.searchParams.get('delimiter')).toBe('/');
    // 潜ると検索欄の入力と確定済みの入力が空になる
    expect(prefixInputOf(container).value).toBe('');

    // 検索欄に入力してからパンくずで戻る
    fireEvent.change(prefixInputOf(container), { target: { value: '2024' } });
    fireEvent.click(breadcrumbButton(container, 'ルート'));

    await waitFor(() => {
      expect(nameCells(container)).toEqual(['logs/', 'readme.txt']);
    });
    expect(prefixInputOf(container).value).toBe('');
    expect(new URL(String(apiCalls(fetchMock)[3][0])).searchParams.get('prefix')).toBe('');

    // 今いるフォルダを示すパンくずの階層は active になる
    fireEvent.click(folderButton(container, 'logs/'));
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['2024/', 'a.txt']);
    });
    expect(breadcrumbButton(container, 'logs').classList.contains('active')).toBe(true);
    expect(breadcrumbButton(container, 'ルート').classList.contains('active')).toBe(false);
  });

  it('階層モードでは今いるフォルダ自身のプレースホルダを一覧から除く', async () => {
    mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'logs/') {
        return jsonResponse({
          objects: [
            // コンソールのフォルダ作成が置く 0 バイトのプレースホルダ (キーが prefix と等しい)
            { key: 'logs/', size: 0, last_modified: '', storage_class: 'STANDARD', etag: 'p' },
            { key: 'logs/a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          ],
          prefixes: [],
          truncated: false,
        });
      }
      return jsonResponse({ objects: [], prefixes: ['logs/'], truncated: false });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'logs/'));

    await waitFor(() => {
      expect(nameCells(container)).toEqual(['a.txt']);
    });
    expect(container.textContent).not.toContain('Objects (2)');
  });

  it('検索欄は今いるフォルダの中の前方一致でサーバへ再取得を要求する', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'logs/rep') {
        return jsonResponse({
          objects: [
            {
              key: 'logs/reports.txt',
              size: 1,
              last_modified: '',
              storage_class: 'STANDARD',
              etag: '1',
            },
          ],
          prefixes: ['logs/reports/'],
          truncated: false,
        });
      }
      if (prefix === 'logs/') {
        return jsonResponse({
          objects: [
            { key: 'logs/a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          ],
          prefixes: [],
          truncated: false,
        });
      }
      return jsonResponse({ objects: [], prefixes: ['logs/'], truncated: false });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'logs/'));
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['a.txt']);
    });

    fireEvent.change(prefixInputOf(container), { target: { value: 'rep' } });
    fireEvent.click(buttonByText(container, '検索'));

    // 前方一致の結果としてフォルダ行とオブジェクト行がどちらも相対名で混在する
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['reports/', 'reports.txt']);
    });
    const searchCall = new URL(String(apiCalls(fetchMock)[2][0]));
    expect(searchCall.searchParams.get('prefix')).toBe('logs/rep');
    expect(searchCall.searchParams.get('delimiter')).toBe('/');
  });

  it('名前の列フィルタはフォルダ行を相対名で絞り、他の列のフィルタはフォルダ行に一致しない', async () => {
    mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'logs/') {
        return jsonResponse({
          objects: [
            {
              key: 'logs/a.txt',
              size: 10,
              last_modified: '',
              storage_class: 'STANDARD',
              etag: '1',
            },
            {
              key: 'logs/logs.txt',
              size: 20,
              last_modified: '',
              storage_class: 'GLACIER',
              etag: '2',
            },
          ],
          prefixes: ['logs/2024/'],
          truncated: false,
        });
      }
      return jsonResponse({ objects: [], prefixes: ['logs/'], truncated: false });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    // フォルダに潜った状態で絞る。ルートでは相対名と完全なキーが同じで、判定の違いが出ない
    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'logs/'));
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['2024/', 'a.txt', 'logs.txt']);
    });

    const filters = Array.from(
      container.querySelectorAll('.dt-filter-row input'),
    ) as HTMLInputElement[];

    // 名前の列は相対名で絞る。完全なキー (logs/2024/、logs/a.txt) で絞ると 3 行とも一致する
    fireEvent.change(filters[0], { target: { value: 'logs' } });
    expect(nameCells(container)).toEqual(['logs.txt']);
    // フォルダ行は相対名 (末尾 "/" 付き) で一致する
    fireEvent.change(filters[0], { target: { value: '2024' } });
    expect(nameCells(container)).toEqual(['2024/']);

    fireEvent.change(filters[0], { target: { value: '' } });
    // 名前以外の列の絞り込みはオブジェクト行で従来どおり効く (フォルダ行はこの列の値を
    // 持たないため一致しない)
    fireEvent.change(filters[3], { target: { value: 'GLACIER' } });
    expect(nameCells(container)).toEqual(['logs.txt']);
  });

  it('名前の列でソートするとフォルダ行とオブジェクト行が完全なキーの順に並ぶ', async () => {
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { key: 'a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          { key: 'c.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '2' },
        ],
        prefixes: ['b/'],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('a.txt');
    });
    // 初期表示はフォルダ行が先頭
    expect(nameCells(container)).toEqual(['b/', 'a.txt', 'c.txt']);

    const nameHeader = container.querySelector('th[data-col-key="key"]') as HTMLTableCellElement;
    fireEvent.click(nameHeader);
    expect(nameCells(container)).toEqual(['a.txt', 'b/', 'c.txt']);

    fireEvent.click(nameHeader);
    expect(nameCells(container)).toEqual(['c.txt', 'b/', 'a.txt']);
  });

  it('トグルでフラットモードに切り替えると delimiter 無しで取得し、今いるフォルダと入力を変えない', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      const u = new URL(url);
      if (u.searchParams.get('delimiter') !== '/') {
        return jsonResponse({
          objects: [
            {
              key: 'logs/2024/a.txt',
              size: 1,
              last_modified: '',
              storage_class: 'STANDARD',
              etag: '1',
            },
          ],
          prefixes: [],
          truncated: false,
        });
      }
      if ((u.searchParams.get('prefix') ?? '') === 'logs/') {
        return jsonResponse({
          objects: [
            { key: 'logs/a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          ],
          prefixes: [],
          truncated: false,
        });
      }
      return jsonResponse({ objects: [], prefixes: ['logs/'], truncated: false });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'logs/'));
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['a.txt']);
    });

    fireEvent.change(prefixInputOf(container), { target: { value: 'zzz' } });
    fireEvent.click(modeButton(container, 'フラット'));

    // 今いるフォルダ以下の全階層を平らに (完全なキーで) 表示する
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['logs/2024/a.txt']);
    });
    const flatCall = new URL(String(apiCalls(fetchMock)[2][0]));
    expect(flatCall.searchParams.get('delimiter')).toBeNull();
    expect(flatCall.searchParams.get('prefix')).toBe('logs/');
    // トグルで今いるフォルダと入力は変わらない
    expect(prefixInputOf(container).value).toBe('zzz');
    expect(breadcrumbButton(container, 'logs').classList.contains('active')).toBe(true);
  });

  it('アップロード先のキーが今いるフォルダと検索欄の入力とファイル名の連結になる', async () => {
    const fetchMock = mockFetchByUrl((url) =>
      url.includes('/objects/upload')
        ? jsonResponse({}, 204)
        : jsonResponse({
            objects: [
              {
                key: 'logs/a.txt',
                size: 1,
                last_modified: '',
                storage_class: 'STANDARD',
                etag: '1',
              },
            ],
            prefixes: ['logs/'],
            truncated: false,
          }),
    );

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'logs/'));
    await waitFor(() => {
      expect(apiCalls(fetchMock)).toHaveLength(2);
    });

    fireEvent.change(prefixInputOf(container), { target: { value: '2024/' } });
    const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: { files: [new File(['hello'], 'hello.txt', { type: 'text/plain' })] },
    });
    fireEvent.click(buttonByText(container, 'Upload'));

    await waitFor(() => {
      expect(apiCalls(fetchMock).length).toBeGreaterThanOrEqual(3);
    });
    expect(String(apiCalls(fetchMock)[2][0])).toContain('key=logs%2F2024%2Fhello.txt');
  });

  it('打ち切りの通知は階層モードとフラットモードで文言が変わる', async () => {
    mockFetchByUrl((url) => {
      const flat = new URL(url).searchParams.get('delimiter') !== '/';
      return jsonResponse({
        objects: [
          { key: 'a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        prefixes: flat ? [] : ['logs/'],
        truncated: true,
      });
    });

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('a.txt');
    });
    expect(container.textContent).toContain('フォルダを開くか');

    fireEvent.click(modeButton(container, 'フラット'));

    await waitFor(() => {
      expect(container.textContent).toContain('prefix で絞り込んで再検索');
    });
    expect(container.textContent).not.toContain('フォルダを開くか');
  });
});
