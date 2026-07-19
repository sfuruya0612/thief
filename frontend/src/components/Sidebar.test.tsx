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

describe('Sidebar SvcItem (queryFn/skipToken)', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('queryFn 未指定による console.error (No queryFn was passed) を出さない', () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    renderWithQC(
      <Sidebar
        profile="test"
        region="ap-northeast-1"
        profiles={[{ name: 'test' }]}
        onRegionChange={() => {}}
        activeService="ec2"
        onService={() => {}}
      />,
    );
    const queryFnErrors = consoleErrorSpy.mock.calls.filter(
      ([msg]) => typeof msg === 'string' && msg.includes('No queryFn was passed'),
    );
    expect(queryFnErrors).toHaveLength(0);
  });

  it('他所で埋まったキャッシュを読み取ってバッジ件数に反映し、自身では fetch しない', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(['aws', 'ec2', 'test', 'ap-northeast-1'], [{}, {}, {}]);

    const { container } = render(
      <QueryClientProvider client={qc}>
        <Sidebar
          profile="test"
          region="ap-northeast-1"
          profiles={[{ name: 'test' }]}
          onRegionChange={() => {}}
          activeService="ec2"
          onService={() => {}}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      const navItems = Array.from(container.querySelectorAll('.nav-item'));
      const ec2Item = navItems.find((el) => el.textContent?.includes('EC2'));
      expect(ec2Item?.querySelector('.count')?.textContent).toBe('3');
    });

    // aws/ec2 の queryKey は skipToken により fetch されないため、実 HTTP リクエストは発生しない
    const ec2FetchCalls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.filter(
      ([url]) => typeof url === 'string' && url.includes('/ec2'),
    );
    expect(ec2FetchCalls).toHaveLength(0);
  });
});
