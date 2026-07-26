import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerELBListeners } from './DrawerELBListeners';

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

function errJson(status: number, code: string, message: string): Response {
  return {
    ok: false,
    status,
    statusText: 'Internal Server Error',
    json: async () => ({ error: message, code }),
  } as Response;
}

const listenerRaw = {
  arn: 'arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:listener/app/my-lb/abc/def',
  load_balancer_arn:
    'arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/my-lb/abc',
  protocol: 'HTTPS',
  port: 443,
  default_action_type: 'forward',
  default_target_group_arn:
    'arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:targetgroup/my-tg/xyz',
};

describe('DrawerELBListeners', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('Listener 一覧の取得エラー時に DrawerError を表示し、テーブルと件数を出さない', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      errJson(500, 'INTERNAL', 'describe listeners failed'),
    );

    const { container } = renderWithQC(
      <DrawerELBListeners profile="test" region="ap-northeast-1" lbArn="arn:lb" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Error 500 (INTERNAL): describe listeners failed');
    });
    // 0 件表示 (テーブルあり + 件数 (0)) と区別できること
    expect(container.querySelector('table')).toBeNull();
    expect(container.querySelector('h3')?.textContent).toBe('Listeners');
  });

  it('Rule 一覧の取得エラー時に Listener テーブルを保ったまま Rules 側に DrawerError を表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(
      async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/elb/listeners')) return okJson([listenerRaw]);
        if (url.includes('/elb/rules')) return errJson(500, 'INTERNAL', 'describe rules failed');
        throw new Error(`unexpected fetch: ${url}`);
      },
    );

    const { container, getByText } = renderWithQC(
      <DrawerELBListeners profile="test" region="ap-northeast-1" lbArn="arn:lb" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('HTTPS');
    });

    fireEvent.click(getByText('HTTPS'));

    await waitFor(() => {
      expect(container.textContent).toContain('Error 500 (INTERNAL): describe rules failed');
    });
    // Listener テーブルは表示されたまま、Rules 側は件数もテーブルも出さない
    expect(container.textContent).toContain('Listeners (1)');
    expect(container.textContent).toContain('HTTPS');
    const rulesHeading = Array.from(container.querySelectorAll('h3')).find((h) =>
      h.textContent?.startsWith('Rules'),
    );
    expect(rulesHeading?.textContent).toBe('Rules');
  });
});
