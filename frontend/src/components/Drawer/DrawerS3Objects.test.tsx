import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerS3Objects } from './DrawerS3Objects';

// テスト間で QueryClient を独立させるためのラッパー
function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('DrawerS3Objects', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('S3 オブジェクト一覧を key/size/storage_class 付きで表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => ({
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
    } as Response);

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
  });

  it('取得件数が上限に達した場合は打ち切りの通知を表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => ({
        objects: [
          { key: 'a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
        ],
        truncated: true,
      }),
    } as Response);

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('a.txt');
    });
    expect(container.textContent).toContain('上限');
  });

  it('ファイル選択とアップロードボタン押下で multipart POST を送る', async () => {
    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    // 1 回目 = 一覧 GET、2 回目 = アップロード POST、3 回目 = 一覧再取得 GET
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({ objects: [], truncated: false }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        statusText: 'No Content',
        json: async () => ({}),
      } as Response)
      .mockResolvedValue({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({ objects: [], truncated: false }),
      } as Response);

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    // 一覧の初回 GET が発火するのを待つ
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
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
      expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
    // 2 回目の呼び出し = アップロード
    const uploadCall = fetchMock.mock.calls[1];
    expect(uploadCall[0]).toContain('/objects/upload');
    expect(uploadCall[0]).toContain('key=hello.txt');
    const init = uploadCall[1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
  });

  it('prefix 入力だけでは再取得されず、検索ボタン押下でサーバへ prefix 付きの再取得を要求する', async () => {
    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({
          objects: [
            { key: 'logs/a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
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
      } as Response)
      .mockResolvedValue({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({
          objects: [
            { key: 'logs/a.txt', size: 1, last_modified: '', storage_class: 'STANDARD', etag: '1' },
          ],
          truncated: false,
        }),
      } as Response);

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('logs/a.txt');
    });
    expect(container.textContent).toContain('other/b.txt');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const prefixInput = container.querySelector(
      'input[placeholder="prefix (folder/subfolder)…"]',
    ) as HTMLInputElement;
    fireEvent.change(prefixInput, { target: { value: '/logs' } });

    // 入力だけでは一覧 GET は再実行されない
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const searchBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === '検索',
    ) as HTMLButtonElement;
    fireEvent.click(searchBtn);

    // 検索ボタン押下でサーバへ prefix 付きの再取得が走る
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    const searchCall = fetchMock.mock.calls[1];
    expect(searchCall[0]).toContain('prefix=logs');

    await waitFor(() => {
      expect(container.textContent).not.toContain('other/b.txt');
    });
    expect(container.textContent).toContain('logs/a.txt');
  });

  it('prefix を入力してアップロードすると key に prefix が付与される', async () => {
    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({ objects: [], truncated: false }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        statusText: 'No Content',
        json: async () => ({}),
      } as Response)
      .mockResolvedValue({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({ objects: [], truncated: false }),
      } as Response);

    const { container } = renderWithQC(
      <DrawerS3Objects profile="test" region="ap-northeast-1" bucket="my-bucket" />,
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    const prefixInput = container.querySelector(
      'input[placeholder="prefix (folder/subfolder)…"]',
    ) as HTMLInputElement;
    fireEvent.change(prefixInput, { target: { value: '/logs/' } });

    // prefix 入力だけでは一覧 GET は再実行されない
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
    fireEvent.change(fileInput, { target: { files: [file] } });

    const uploadBtn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'Upload',
    ) as HTMLButtonElement;
    fireEvent.click(uploadBtn);

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
    const uploadCall = fetchMock.mock.calls[1];
    expect(uploadCall[0]).toContain('/objects/upload');
    expect(uploadCall[0]).toContain('key=logs%2Fhello.txt');
  });

  it('バイナリ拡張子や 5 MB 以上のオブジェクトは Preview ボタンを無効化し行をグレーアウトする', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => ({
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
    } as Response);

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
    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({
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
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({ content: 'hello preview', content_type: 'text/plain', size: 13 }),
      } as Response);

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

    const previewCall = fetchMock.mock.calls[1];
    expect(previewCall[0]).toContain('/objects/preview');
    expect(previewCall[0]).toContain('key=notes.txt');
  });

  it('プレビューを編集して保存すると同じキーへ upload し確認ダイアログを挟む', async () => {
    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({
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
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => ({ content: 'hello preview', content_type: 'text/plain', size: 13 }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        statusText: 'No Content',
        json: async () => ({}),
      } as Response);

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
      expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(3);
    });
    const saveCall = fetchMock.mock.calls[2];
    expect(saveCall[0]).toContain('/objects/upload');
    expect(saveCall[0]).toContain('key=notes.txt');
    const init = saveCall[1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);

    confirmSpy.mockRestore();
  });
});
