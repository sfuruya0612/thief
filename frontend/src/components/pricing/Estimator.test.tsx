import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Estimator } from './Estimator';
import type { PriceRateRow, PriceTableRow } from '../../types/aws';
import type { PriceSelectionByService, PriceTablesByService } from '../../lib/pricingEstimate';

function rate(overrides: Partial<PriceRateRow> = {}): PriceRateRow {
  return {
    rateId: 'sku.1',
    model: 'on_demand',
    group: 'On-Demand',
    label: 'm5.large / Linux / Shared',
    attributes: { instance_type: 'm5.large', os: 'Linux' },
    term: { lease: null, offeringClass: null, payment: null },
    unit: 'Hrs',
    priceUSD: 0.096,
    upfrontUSD: 0,
    currency: 'USD',
    ...overrides,
  };
}

function table(overrides: Partial<PriceTableRow> = {}): PriceTableRow {
  return {
    service: 'ec2',
    region: 'ap-northeast-1',
    fetchedAt: '2026-07-18T09:00:00Z',
    licenseUnresolved: false,
    rates: [rate()],
    ...overrides,
  };
}

function renderEstimator(props: Partial<Parameters<typeof Estimator>[0]> = {}) {
  const onSetQty = props.onSetQty ?? vi.fn();
  const onToggleRate = props.onToggleRate ?? vi.fn();
  const onClearAll = props.onClearAll ?? vi.fn();
  const view = render(
    <Estimator
      selection={props.selection ?? {}}
      rates={props.rates ?? {}}
      onSetQty={onSetQty}
      onToggleRate={onToggleRate}
      onClearAll={onClearAll}
    />,
  );
  return { ...view, onSetQty, onToggleRate, onClearAll };
}

describe('Estimator / 一括削除', () => {
  it('見積もりが空のときは一括削除ボタンが無効になる', () => {
    renderEstimator();
    expect(screen.getByRole('button', { name: '一括削除' })).toBeDisabled();
  });

  it('見積もりに項目があるときは一括削除ボタンが有効になる', () => {
    const selection: PriceSelectionByService = { ec2: { 'sku.1': { checked: true, qty: 2 } } };
    const rates: PriceTablesByService = { ec2: table() };
    renderEstimator({ selection, rates });
    expect(screen.getByRole('button', { name: '一括削除' })).not.toBeDisabled();
  });

  it('確認ダイアログで OK すると onClearAll が呼ばれる', () => {
    const selection: PriceSelectionByService = { ec2: { 'sku.1': { checked: true, qty: 2 } } };
    const rates: PriceTablesByService = { ec2: table() };
    const { onClearAll } = renderEstimator({ selection, rates });

    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    fireEvent.click(screen.getByRole('button', { name: '一括削除' }));

    expect(confirmSpy).toHaveBeenCalledTimes(1);
    expect(onClearAll).toHaveBeenCalledTimes(1);
    confirmSpy.mockRestore();
  });

  it('確認ダイアログでキャンセルすると onClearAll は呼ばれない', () => {
    const selection: PriceSelectionByService = { ec2: { 'sku.1': { checked: true, qty: 2 } } };
    const rates: PriceTablesByService = { ec2: table() };
    const { onClearAll } = renderEstimator({ selection, rates });

    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    fireEvent.click(screen.getByRole('button', { name: '一括削除' }));

    expect(onClearAll).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });
});

describe('Estimator / 注記', () => {
  it('730 時間/月の近似の説明のみを表示し、実月の時間数・Savings Plans 前払いの注記は表示しない', () => {
    renderEstimator();
    expect(
      screen.getByText('730 時間/月 (365×24/12 の近似) で計算しています。'),
    ).toBeInTheDocument();
    expect(screen.queryByText(/実月の時間数/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Savings Plans の前払い/)).not.toBeInTheDocument();
  });
});

describe('Estimator / 期間・購入タイプの表示 (issue 0051)', () => {
  it('Reserved Instance の行に期間・オファリングクラス・購入タイプを表示する', () => {
    const riRate = rate({
      rateId: 'sku.ri',
      model: 'reserved',
      label: 'm5.large / Linux / Shared',
      term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
    });
    const selection: PriceSelectionByService = { ec2: { 'sku.ri': { checked: true, qty: 1 } } };
    const rates: PriceTablesByService = { ec2: table({ rates: [riRate] }) };
    renderEstimator({ selection, rates });
    expect(screen.getByText('1yr / standard / No Upfront')).toBeInTheDocument();
  });

  it('Savings Plans の行に期間・購入タイプを表示する (オファリングクラスは持たない)', () => {
    const spRate = rate({
      rateId: 'sku.sp',
      model: 'savings_plan',
      label: 'm5.large / Compute Savings Plans',
      term: { lease: '3yr', offeringClass: null, payment: 'All Upfront' },
    });
    const selection: PriceSelectionByService = { ec2: { 'sku.sp': { checked: true, qty: 1 } } };
    const rates: PriceTablesByService = { ec2: table({ rates: [spRate] }) };
    renderEstimator({ selection, rates });
    expect(screen.getByText('3yr / All Upfront')).toBeInTheDocument();
  });

  it('同一インスタンスタイプで期間の異なる RI を複数チェックすると、明細で区別できる', () => {
    const ri1yr = rate({
      rateId: 'sku.ri.1yr',
      model: 'reserved',
      term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
    });
    const ri3yr = rate({
      rateId: 'sku.ri.3yr',
      model: 'reserved',
      term: { lease: '3yr', offeringClass: 'standard', payment: 'No Upfront' },
    });
    const selection: PriceSelectionByService = {
      ec2: { 'sku.ri.1yr': { checked: true, qty: 1 }, 'sku.ri.3yr': { checked: true, qty: 1 } },
    };
    const rates: PriceTablesByService = { ec2: table({ rates: [ri1yr, ri3yr] }) };
    renderEstimator({ selection, rates });
    expect(screen.getByText('1yr / standard / No Upfront')).toBeInTheDocument();
    expect(screen.getByText('3yr / standard / No Upfront')).toBeInTheDocument();
  });

  it('On-Demand の行には期間・購入タイプを表示しない', () => {
    const selection: PriceSelectionByService = { ec2: { 'sku.1': { checked: true, qty: 1 } } };
    const rates: PriceTablesByService = { ec2: table() };
    const { container } = renderEstimator({ selection, rates });
    expect(container.querySelector('.pr-estimator-line-term')).toBeNull();
  });
});
