// DrawerGCSObjects のテスト。描画本体は DrawerObjectBrowser に共通化されているため、GCS 固有の
// 注入 (SSO 期限切れバナーに使う profile を渡さないこと) と、S3 と同じ Query ボタンが出ること、
// 階層モードの一覧 (フォルダ行、遷移、検索、列フィルタ、ソート、トグル、アップロード先、
// 打ち切りの通知) を GCS の Raw 型 (name / updated) で検証する。アップロードとプレビューの
// 検証は DrawerS3Objects.test.tsx が担う。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerGCSObjects } from './DrawerGCSObjects';

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

const OBJECTS = {
  objects: [
    { name: 'data.csv', size: 100, updated: '', storage_class: 'STANDARD', content_type: '' },
    { name: 'app.log', size: 100, updated: '', storage_class: 'STANDARD', content_type: '' },
    {
      name: 'huge.csv',
      size: (1 << 30) + 1,
      updated: '',
      storage_class: 'STANDARD',
      content_type: '',
    },
  ],
  prefixes: [],
  truncated: false,
};

function objectQueryButtons(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll('button')).filter(
    (b) => b.textContent === 'Query',
  ) as HTMLButtonElement[];
}

// mockFetchByUrl は /api/config 以外を handler が URL ごとに返すモックに差し替える。
// handler が undefined を返した URL への呼び出しはテストの誤りとして例外にする。
function mockFetchByUrl(handler: (url: string) => Response | undefined) {
  const mock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes('/api/config')) return jsonResponse(CONFIG_RESPONSE);
    const res = handler(url);
    if (!res) throw new Error(`unexpected fetch: ${url}`);
    return res;
  });
  globalThis.fetch = mock as unknown as typeof fetch;
  return mock;
}

// apiCalls は /api/config 以外の呼び出しだけを呼び出し順に返す。
function apiCalls(fetchMock: ReturnType<typeof vi.fn>): unknown[][] {
  return fetchMock.mock.calls.filter((call) => !String(call[0]).includes('/api/config'));
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

describe('DrawerGCSObjects', () => {
  const originalFetch = globalThis.fetch;
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');
  const originalLocks = Object.getOwnPropertyDescriptor(globalThis.navigator, 'locks');

  beforeEach(() => {
    // Query ボタンの活性判定が要求するブラウザ機能 (OPFS と Web Locks) を jsdom に与える
    Object.defineProperty(globalThis.navigator, 'storage', {
      value: { getDirectory: () => Promise.resolve() },
      configurable: true,
    });
    Object.defineProperty(globalThis.navigator, 'locks', {
      value: { request: () => Promise.resolve() },
      configurable: true,
    });
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/config')) return jsonResponse(CONFIG_RESPONSE);
      if (url.includes('/api/gcp/gcs/')) return jsonResponse(OBJECTS);
      throw new Error(`unexpected fetch: ${url}`);
    }) as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
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

  it('Query ボタンを S3 と同じ条件で出し分ける', async () => {
    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    await waitFor(() => {
      expect(container.textContent).toContain('data.csv');
    });
    // 設定の取得完了を待つ (取得完了までは Query が無効)
    await waitFor(() => {
      expect(objectQueryButtons(container)[0].disabled).toBe(false);
    });

    const [csvBtn, logBtn, hugeBtn] = objectQueryButtons(container);
    expect(csvBtn.disabled).toBe(false);
    expect(csvBtn.title).toBe('オブジェクトに SQL を実行');
    expect(logBtn.disabled).toBe(true);
    expect(logBtn.title).toBe('SQL 検索の対象外の形式です');
    expect(hugeBtn.disabled).toBe(true);
    expect(hugeBtn.title).toBe('サイズ上限 (1.0 GiB) を超えるオブジェクトは SQL 検索できません');
  });

  it('初期表示はバケットのルートを階層モードで取得し、フォルダ行を名前の列だけで描く', async () => {
    const fetchMock = mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          {
            name: 'readme.txt',
            size: 10,
            content_type: 'text/plain',
            storage_class: 'STANDARD',
            updated: '',
          },
        ],
        prefixes: ['logs/', 'reports/'],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    await waitFor(() => {
      expect(container.textContent).toContain('readme.txt');
    });

    // 初回はバケットのルートを階層モード (delimiter=/) で取得する
    const firstCall = new URL(String(apiCalls(fetchMock)[0][0]));
    expect(firstCall.searchParams.get('delimiter')).toBe('/');
    expect(firstCall.searchParams.get('project_id')).toBe('p');

    // ソートしていない初期表示はフォルダ行が先頭、件数はオブジェクトのみ
    expect(nameCells(container)).toEqual(['logs/', 'reports/', 'readme.txt']);
    expect(container.textContent).toContain('Objects (1)');

    const folderRow = dataRows(container)[0];
    const cells = Array.from(folderRow.querySelectorAll('td'));
    expect(cells[1].querySelector('button')).not.toBeNull();
    // 名前以外の列 (size / content type / storage class / updated) と Actions 列は空
    for (const cell of cells.slice(2)) {
      expect(cell.textContent).toBe('');
      expect(cell.querySelector('button')).toBeNull();
      expect(cell.querySelector('a')).toBeNull();
    }
    expect(folderRow.classList.contains('preview-ineligible')).toBe(false);
  });

  it('フォルダ行のクリックで潜り、パンくずで戻る。どちらも検索欄と確定済みの入力が空になる', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'logs/') {
        return jsonResponse({
          objects: [
            // プレースホルダ (キーが prefix と等しい) は一覧から除く
            { name: 'logs/', size: 0, content_type: '', storage_class: 'STANDARD', updated: '' },
            {
              name: 'logs/a.txt',
              size: 1,
              content_type: 'text/plain',
              storage_class: 'STANDARD',
              updated: '',
            },
          ],
          prefixes: ['logs/2024/'],
          truncated: false,
        });
      }
      return jsonResponse({
        objects: [],
        prefixes: ['logs/'],
        truncated: false,
      });
    });

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
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
    // 相対名で表示し、完全なキーは出さない
    expect(container.textContent).not.toContain('logs/a.txt');
    expect(prefixInputOf(container).value).toBe('');
    expect(new URL(String(apiCalls(fetchMock)[2][0])).searchParams.get('prefix')).toBe('logs/');

    fireEvent.change(prefixInputOf(container), { target: { value: '2024' } });
    fireEvent.click(breadcrumbButton(container, 'ルート'));

    await waitFor(() => {
      expect(nameCells(container)).toEqual(['logs/']);
    });
    expect(prefixInputOf(container).value).toBe('');
    expect(breadcrumbButton(container, 'ルート').classList.contains('active')).toBe(true);
  });

  it('検索欄は今いるフォルダの中の前方一致で再取得を要求する', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'logs/rep') {
        return jsonResponse({
          objects: [
            {
              name: 'logs/reports.txt',
              size: 1,
              content_type: '',
              storage_class: 'STANDARD',
              updated: '',
            },
          ],
          prefixes: ['logs/reports/'],
          truncated: false,
        });
      }
      if (prefix === 'logs/') {
        return jsonResponse({
          objects: [
            {
              name: 'logs/a.txt',
              size: 1,
              content_type: '',
              storage_class: 'STANDARD',
              updated: '',
            },
          ],
          prefixes: [],
          truncated: false,
        });
      }
      return jsonResponse({ objects: [], prefixes: ['logs/'], truncated: false });
    });

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    await waitFor(() => {
      expect(folderButton(container, 'logs/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'logs/'));
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['a.txt']);
    });

    fireEvent.change(prefixInputOf(container), { target: { value: 'rep' } });
    fireEvent.click(buttonByText(container, '検索'));

    await waitFor(() => {
      expect(nameCells(container)).toEqual(['reports/', 'reports.txt']);
    });
    expect(new URL(String(apiCalls(fetchMock)[2][0])).searchParams.get('prefix')).toBe('logs/rep');
  });

  it('名前の列フィルタはフォルダ行を相対名で絞る', async () => {
    mockFetchByUrl((url) => {
      const prefix = new URL(url).searchParams.get('prefix') ?? '';
      if (prefix === 'b/') {
        return jsonResponse({
          objects: [
            { name: 'b/b.csv', size: 10, content_type: '', storage_class: 'STANDARD', updated: '' },
            { name: 'b/x.csv', size: 20, content_type: '', storage_class: 'STANDARD', updated: '' },
          ],
          prefixes: ['b/bb/'],
          truncated: false,
        });
      }
      return jsonResponse({ objects: [], prefixes: ['b/'], truncated: false });
    });

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    // フォルダに潜った状態で絞る。ルートでは相対名と完全なキーが同じで、判定の違いが出ない
    await waitFor(() => {
      expect(folderButton(container, 'b/')).toBeDefined();
    });
    fireEvent.click(folderButton(container, 'b/'));
    await waitFor(() => {
      expect(nameCells(container)).toEqual(['bb/', 'b.csv', 'x.csv']);
    });

    const filters = Array.from(
      container.querySelectorAll('.dt-filter-row input'),
    ) as HTMLInputElement[];
    // 完全なキー (b/bb/、b/b.csv、b/x.csv) で絞ると 3 行とも一致する
    fireEvent.change(filters[0], { target: { value: 'b' } });
    expect(nameCells(container)).toEqual(['bb/', 'b.csv']);
  });

  it('名前の列でソートするとフォルダ行とオブジェクト行が完全なキーの順に並ぶ', async () => {
    mockFetchByUrl(() =>
      jsonResponse({
        objects: [
          { name: 'a.csv', size: 10, content_type: '', storage_class: 'STANDARD', updated: '' },
          { name: 'c.csv', size: 20, content_type: '', storage_class: 'STANDARD', updated: '' },
        ],
        prefixes: ['b/'],
        truncated: false,
      }),
    );

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    await waitFor(() => {
      expect(container.textContent).toContain('a.csv');
    });

    // 名前の列 (name) でソートすると、フォルダ行は完全な prefix としてオブジェクト行と混ざる
    const nameHeader = container.querySelector('th[data-col-key="name"]') as HTMLTableCellElement;
    fireEvent.click(nameHeader);
    expect(nameCells(container)).toEqual(['a.csv', 'b/', 'c.csv']);
    fireEvent.click(nameHeader);
    expect(nameCells(container)).toEqual(['c.csv', 'b/', 'a.csv']);
  });

  it('トグルでフラットモードに切り替えると delimiter 無しで取得し、打ち切りの文言が変わる', async () => {
    const fetchMock = mockFetchByUrl((url) => {
      const u = new URL(url);
      if (u.searchParams.get('delimiter') !== '/') {
        return jsonResponse({
          objects: [
            {
              name: 'logs/2024/a.txt',
              size: 1,
              content_type: '',
              storage_class: 'STANDARD',
              updated: '',
            },
          ],
          prefixes: [],
          truncated: true,
        });
      }
      return jsonResponse({
        objects: [
          { name: 'logs/a.txt', size: 1, content_type: '', storage_class: 'STANDARD', updated: '' },
        ],
        prefixes: ['logs/'],
        truncated: true,
      });
    });

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

    await waitFor(() => {
      expect(container.textContent).toContain('logs/a.txt');
    });
    expect(container.textContent).toContain('フォルダを開くか');

    fireEvent.click(modeButton(container, 'フラット'));

    await waitFor(() => {
      expect(nameCells(container)).toEqual(['logs/2024/a.txt']);
    });
    const flatCall = new URL(String(apiCalls(fetchMock)[1][0]));
    expect(flatCall.searchParams.get('delimiter')).toBeNull();
    expect(container.textContent).toContain('prefix で絞り込んで再検索');
    expect(container.textContent).not.toContain('フォルダを開くか');
  });

  it('アップロード先のキーが今いるフォルダと検索欄の入力とファイル名の連結になる', async () => {
    const fetchMock = mockFetchByUrl((url) =>
      url.includes('/objects/upload')
        ? jsonResponse({}, 204)
        : jsonResponse({
            objects: [],
            prefixes: ['logs/'],
            truncated: false,
          }),
    );

    const { container } = renderWithQC(<DrawerGCSObjects projectId="p" bucket="my-bucket" />);

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
});
