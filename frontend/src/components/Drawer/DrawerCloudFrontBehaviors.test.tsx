import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerCloudFrontBehaviors } from './DrawerCloudFrontBehaviors';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function okJson(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    json: async () => body,
  } as Response;
}

function cloudfrontRaw(behaviors: unknown) {
  return {
    id: 'E123',
    name: 'test',
    state: 'deployed',
    domain_name: 'd123.cloudfront.net',
    aliases: null,
    origins: null,
    behaviors,
    enabled: true,
    price_class: 'PriceClass_All',
    cost_monthly: 0,
  };
}

const additionalBehaviorRaw = {
  path_pattern: '/api/*',
  target_origin_id: 'origin-2',
  viewer_protocol_policy: 'allow-all',
  allowed_methods: ['GET', 'HEAD'],
  compress: false,
  is_default: false,
};

const defaultBehaviorRaw = {
  path_pattern: '',
  target_origin_id: 'origin-default',
  viewer_protocol_policy: 'redirect-to-https',
  allowed_methods: ['GET', 'HEAD'],
  compress: true,
  is_default: true,
};

describe('DrawerCloudFrontBehaviors', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('一覧未取得かつ該当行が無い間は No behaviors. ではなくローディングを表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockReturnValue(new Promise(() => {}));

    const { container } = renderWithQC(
      <DrawerCloudFrontBehaviors profile="test" region="ap-northeast-1" id="E123" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Loading…');
    });
    expect(container.textContent).not.toContain('No behaviors.');
  });

  it('一覧取得済みで該当行が無いときは Loading… ではなく No behaviors. を表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(okJson([]));

    const { container } = renderWithQC(
      <DrawerCloudFrontBehaviors profile="test" region="ap-northeast-1" id="E123" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('No behaviors.');
    });
    expect(container.textContent).not.toContain('Loading…');
  });

  it('behaviors が空配列のとき No behaviors. を表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(okJson([cloudfrontRaw([])]));

    const { container } = renderWithQC(
      <DrawerCloudFrontBehaviors profile="test" region="ap-northeast-1" id="E123" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('No behaviors.');
    });
  });

  it('既定ビヘイビアのみのとき 1 件表示され、No behaviors. にならない', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      okJson([cloudfrontRaw([defaultBehaviorRaw])]),
    );

    const { container } = renderWithQC(
      <DrawerCloudFrontBehaviors profile="test" region="ap-northeast-1" id="E123" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Behaviors (1)');
    });
    expect(container.textContent).not.toContain('No behaviors.');
  });

  it('既定ビヘイビアが Default (*) 表示で末尾に並ぶ', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      okJson([cloudfrontRaw([additionalBehaviorRaw, defaultBehaviorRaw])]),
    );

    const { container } = renderWithQC(
      <DrawerCloudFrontBehaviors profile="test" region="ap-northeast-1" id="E123" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Default (*)');
    });
    const rows = Array.from(container.querySelectorAll('tbody tr'));
    expect(rows.length).toBe(2);
    expect(rows[rows.length - 1].textContent).toContain('Default (*)');
  });
});
