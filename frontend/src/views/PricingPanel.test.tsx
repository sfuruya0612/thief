import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PricingPanel } from './PricingPanel';
import * as endpoints from '../api/endpoints';
import { ApiError } from '../types/common';
import type { PriceRateRaw, PriceTableRaw } from '../types/aws';

function rate(overrides: Partial<PriceRateRaw> = {}): PriceRateRaw {
  return {
    rate_id: 'sku.EC2.ondemand.m5large',
    model: 'on_demand',
    group: 'On-Demand',
    label: 'm5.large / Linux / Shared',
    attributes: { instance_type: 'm5.large', os: 'Linux' },
    term: { lease: null, offering_class: null, payment: null },
    unit: 'Hrs',
    price_usd: 0.096,
    upfront_usd: 0,
    currency: 'USD',
    ...overrides,
  };
}

function table(service: string, overrides: Partial<PriceTableRaw> = {}): PriceTableRaw {
  return {
    service,
    region: 'ap-northeast-1',
    fetched_at: '2026-07-18T09:00:00Z',
    partial: false,
    missing_models: [],
    rates: [],
    ...overrides,
  };
}

function cardFor(labelText: string): HTMLElement {
  const title = screen.getByText(labelText, { selector: '.pr-card-title' });
  const card = title.closest('.pr-card');
  if (!card) throw new Error(`card not found: ${labelText}`);
  return card as HTMLElement;
}

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <PricingPanel profile="test-profile" region="ap-northeast-1" onRegionChange={() => {}} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.spyOn(endpoints, 'getRegions').mockResolvedValue([]);
});

describe('PricingPanel', () => {
  it('取得中のサービスはローディング表示になる (loading)', async () => {
    vi.spyOn(endpoints, 'getPricing').mockImplementation(async (_profile, _region, service) => {
      if (service === 'ec2') return new Promise<PriceTableRaw>(() => undefined);
      return table(service);
    });
    renderPanel();

    await waitFor(() => {
      expect(within(cardFor('RDS')).queryByText('Loading…')).not.toBeInTheDocument();
    });
    expect(within(cardFor('EC2')).getByText('Loading…')).toBeInTheDocument();
  });

  it('取得済みデータはレート行として表示される (ready)', async () => {
    vi.spyOn(endpoints, 'getPricing').mockImplementation(async (_profile, _region, service) => {
      if (service === 'ec2') return table('ec2', { rates: [rate()] });
      return table(service);
    });
    renderPanel();

    await waitFor(() => {
      expect(within(cardFor('EC2')).getByText('m5.large / Linux / Shared')).toBeInTheDocument();
    });
  });

  it('更新ボタン押下中はキャッシュ表示のまま「更新中」バッジが出る (stale)', async () => {
    let resolveRefresh: ((v: PriceTableRaw) => void) | undefined;
    vi.spyOn(endpoints, 'getPricing').mockImplementation(
      async (_profile, _region, service, refresh) => {
        if (service === 'ec2' && refresh) {
          return new Promise<PriceTableRaw>((resolve) => {
            resolveRefresh = resolve;
          });
        }
        return table(service, service === 'ec2' ? { rates: [rate()] } : {});
      },
    );
    renderPanel();

    const ec2Card = await waitFor(() => {
      const card = cardFor('EC2');
      expect(within(card).getByText('m5.large / Linux / Shared')).toBeInTheDocument();
      return card;
    });

    fireEvent.click(within(ec2Card).getByTitle('このサービスの単価を再取得する'));

    await waitFor(() => expect(within(ec2Card).getByText('更新中…')).toBeInTheDocument());

    resolveRefresh?.(table('ec2', { rates: [rate()] }));
    await waitFor(() => expect(within(ec2Card).queryByText('更新中…')).not.toBeInTheDocument());
  });

  it('キャッシュのないエラーはエラーバナーと再試行ボタンを表示する (error)', async () => {
    vi.spyOn(endpoints, 'getPricing').mockImplementation(async (_profile, _region, service) => {
      if (service === 'ec2') {
        throw new ApiError(403, 'PRICING_ACCESS_DENIED', 'missing iam permission');
      }
      return table(service);
    });
    renderPanel();

    const ec2Card = await waitFor(() => {
      const card = cardFor('EC2');
      expect(within(card).getByText('missing iam permission')).toBeInTheDocument();
      return card;
    });
    expect(within(ec2Card).getByText('再試行')).toBeInTheDocument();
  });

  it('rates が空のテーブルは該当する単価がない旨を表示する (empty)', async () => {
    vi.spyOn(endpoints, 'getPricing').mockImplementation(async (_profile, _region, service) =>
      table(service),
    );
    renderPanel();

    await waitFor(() => {
      expect(within(cardFor('EC2')).getByText('該当する単価がありません。')).toBeInTheDocument();
    });
  });

  it('partial なテーブルは Savings Plans 取得失敗を明示する (partial)', async () => {
    vi.spyOn(endpoints, 'getPricing').mockImplementation(async (_profile, _region, service) => {
      if (service === 'rds') {
        return table('rds', {
          partial: true,
          missing_models: ['savings_plan'],
          rates: [rate({ rate_id: 'sku.rds', group: 'On-Demand', label: 'db.m5.large / MySQL' })],
        });
      }
      return table(service);
    });
    renderPanel();

    const rdsCard = await waitFor(() => {
      const card = cardFor('RDS');
      expect(within(card).getByText('db.m5.large / MySQL')).toBeInTheDocument();
      return card;
    });
    expect(within(rdsCard).getByText('Savings Plans 取得失敗 (縮退表示)')).toBeInTheDocument();
    expect(
      within(rdsCard).getByText(
        'Savings Plans の取得に失敗したため、On-Demand / Reserved Instance のみ表示しています。',
      ),
    ).toBeInTheDocument();
  });
});
