import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerCFNEvents } from './DrawerCFNEvents';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('DrawerCFNEvents', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('失敗系ステータス (CREATE_FAILED / ROLLBACK 系) の行を赤色で表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => [
        {
          timestamp: '2026-07-02T00:00:00Z',
          logical_resource_id: 'MyBucket',
          resource_type: 'AWS::S3::Bucket',
          resource_status: 'CREATE_FAILED',
          resource_status_reason: 'Bucket already exists',
        },
        {
          timestamp: '2026-07-02T00:00:01Z',
          logical_resource_id: 'MyRole',
          resource_type: 'AWS::IAM::Role',
          resource_status: 'ROLLBACK_IN_PROGRESS',
          resource_status_reason: '',
        },
        {
          timestamp: '2026-07-02T00:00:02Z',
          logical_resource_id: 'MyTopic',
          resource_type: 'AWS::SNS::Topic',
          resource_status: 'CREATE_COMPLETE',
          resource_status_reason: '',
        },
      ],
    } as Response);

    const { container } = renderWithQC(
      <DrawerCFNEvents profile="test" region="ap-northeast-1" stack="my-stack" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('MyBucket');
    });

    const findCell = (text: string) =>
      Array.from(container.querySelectorAll('td')).find((td) => td.textContent === text);

    const failedCell = findCell('CREATE_FAILED');
    const rollbackCell = findCell('ROLLBACK_IN_PROGRESS');
    const completeCell = findCell('CREATE_COMPLETE');

    expect(failedCell?.querySelector('span')).toHaveStyle({ color: 'var(--err)' });
    expect(rollbackCell?.querySelector('span')).toHaveStyle({ color: 'var(--err)' });
    expect(completeCell?.querySelector('span')?.getAttribute('style') ?? '').not.toContain(
      'var(--err)',
    );
  });

  it('取得エラー時に DrawerError を表示し、テーブルと件数を出さない', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: 'describe stack events failed', code: 'INTERNAL' }),
    } as Response);

    const { container } = renderWithQC(
      <DrawerCFNEvents profile="test" region="ap-northeast-1" stack="my-stack" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Error 500 (INTERNAL): describe stack events failed');
    });
    // 0 件表示 (テーブルあり + 件数 (0)) と区別できること
    expect(container.querySelector('table')).toBeNull();
    expect(container.querySelector('h3')?.textContent).toBe('Events');
  });
});
