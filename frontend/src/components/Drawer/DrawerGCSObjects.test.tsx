// DrawerGCSObjects のテスト。描画本体は DrawerObjectBrowser に共通化されているため、ここでは
// GCS 固有の注入 (SSO 期限切れバナーに使う profile を渡さないこと) と、S3 と同じ Query ボタンが
// 出ることだけを検証する。一覧・アップロード・プレビューの検証は DrawerS3Objects.test.tsx が担う。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
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
  truncated: false,
};

function objectQueryButtons(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll('button')).filter(
    (b) => b.textContent === 'Query',
  ) as HTMLButtonElement[];
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
});
