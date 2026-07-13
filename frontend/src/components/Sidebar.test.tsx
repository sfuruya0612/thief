import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Sidebar } from './Sidebar';

// テスト間で QueryClient を独立させるためのラッパー
function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('Sidebar region selector', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  function mockRegionsResponse(regions: { code: string; name: string }[]) {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => regions,
    } as Response);
  }

  it('取得前は現在の region 値を単一オプションで表示する', () => {
    // fetch は解決せず (取得前状態) にする
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(() => new Promise(() => {}));
    const { container } = renderWithQC(
      <Sidebar
        profile="test"
        region="ap-northeast-1"
        profiles={[{ name: 'test' }]}
        onProfileChange={() => {}}
        onRegionChange={() => {}}
        activeService="ec2"
        onService={() => {}}
      />,
    );
    const regionSelect = container.querySelectorAll('select')[0];
    const options = regionSelect.querySelectorAll('option');
    expect(options).toHaveLength(1);
    expect(options[0].getAttribute('value')).toBe('ap-northeast-1');
    expect(options[0].textContent).toBe('ap-northeast-1');
  });

  it('取得後は "名前 (コード)" 表記でリージョン一覧を表示する', async () => {
    mockRegionsResponse([
      { code: 'us-east-1', name: 'US East (N. Virginia)' },
      { code: 'ap-northeast-1', name: 'Asia Pacific (Tokyo)' },
    ]);
    const { container } = renderWithQC(
      <Sidebar
        profile="test"
        region="ap-northeast-1"
        profiles={[{ name: 'test' }]}
        onProfileChange={() => {}}
        onRegionChange={() => {}}
        activeService="ec2"
        onService={() => {}}
      />,
    );
    await waitFor(() => {
      const regionSelect = container.querySelectorAll('select')[0];
      const options = regionSelect.querySelectorAll('option');
      expect(options).toHaveLength(2);
    });
    const regionSelect = container.querySelectorAll('select')[0];
    const optionTexts = Array.from(regionSelect.querySelectorAll('option')).map(
      (o) => o.textContent,
    );
    expect(optionTexts).toContain('US East (N. Virginia) (us-east-1)');
    expect(optionTexts).toContain('Asia Pacific (Tokyo) (ap-northeast-1)');
  });
});
