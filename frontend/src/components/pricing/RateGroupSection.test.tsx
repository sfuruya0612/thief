import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import type { PriceRateRow } from '../../types/aws';
import { RateGroupSection } from './RateGroupSection';

function rate(overrides: Partial<PriceRateRow> = {}): PriceRateRow {
  return {
    rateId: 'sku.default',
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

function renderGroup(props: Partial<Parameters<typeof RateGroupSection>[0]> = {}) {
  const onToggleRate = props.onToggleRate ?? vi.fn();
  return render(
    <RateGroupSection
      group={props.group ?? 'On-Demand'}
      rates={props.rates ?? [rate()]}
      selection={props.selection ?? {}}
      onToggleRate={onToggleRate}
      instanceFilter={props.instanceFilter ?? ''}
      hideTitle={props.hideTitle}
    />,
  );
}

describe('RateGroupSection', () => {
  it('On-Demand は条件セレクタなしで全行を表示する', () => {
    renderGroup({
      group: 'On-Demand',
      rates: [
        rate({ rateId: 'a', label: 'm5.large / Linux' }),
        rate({ rateId: 'b', label: 'm5.xlarge / Linux' }),
      ],
    });

    expect(screen.getByText('m5.large / Linux')).toBeInTheDocument();
    expect(screen.getByText('m5.xlarge / Linux')).toBeInTheDocument();
    expect(document.querySelector('.pr-group-conditions')).toBeNull();
  });

  it('RI は lease/offeringClass/payment の全条件が揃うとセレクタで1組に絞り込む', () => {
    const rates = [
      rate({
        rateId: 'std-1yr-noupfront',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'm5.large standard 1yr No Upfront',
        term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
      rate({
        rateId: 'std-3yr-noupfront',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'm5.large standard 3yr No Upfront',
        term: { lease: '3yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
      rate({
        rateId: 'convertible-1yr-noupfront',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'm5.large convertible 1yr No Upfront',
        term: { lease: '1yr', offeringClass: 'convertible', payment: 'No Upfront' },
      }),
    ];
    renderGroup({ group: 'Reserved Instance', rates });

    // デフォルトは各条件の先頭値 (1yr / standard / No Upfront) の組み合わせのみ表示
    expect(screen.getByText('m5.large standard 1yr No Upfront')).toBeInTheDocument();
    expect(screen.queryByText('m5.large standard 3yr No Upfront')).not.toBeInTheDocument();
    expect(screen.queryByText('m5.large convertible 1yr No Upfront')).not.toBeInTheDocument();

    // offeringClass/payment (standard / No Upfront) は固定したまま lease だけ切り替える
    const leaseSelect = screen.getByDisplayValue('1yr');
    fireEvent.change(leaseSelect, { target: { value: '3yr' } });

    expect(screen.getByText('m5.large standard 3yr No Upfront')).toBeInTheDocument();
    expect(screen.queryByText('m5.large standard 1yr No Upfront')).not.toBeInTheDocument();
  });

  it('offeringClass が単一値しかない場合はセレクタを出さず全行を素通しする (RDS/ElastiCache RI)', () => {
    const rates = [
      rate({
        rateId: 'rds-1yr',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'db.m5.large 1yr No Upfront',
        term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
      rate({
        rateId: 'rds-3yr',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'db.m5.large 3yr No Upfront',
        term: { lease: '3yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
    ];
    renderGroup({ group: 'Reserved Instance', rates });

    // offeringClass は standard のみのためセレクタは出ない (1 個の select は lease 用のみ)
    expect(document.querySelectorAll('.pr-group-conditions select')).toHaveLength(1);
    expect(screen.getByText('db.m5.large 1yr No Upfront')).toBeInTheDocument();
  });

  it('instanceFilter は label と attributes の両方に部分一致する', () => {
    renderGroup({
      rates: [
        rate({ rateId: 'a', label: 'm5.large / Linux', attributes: { instance_type: 'm5.large' } }),
        rate({ rateId: 'b', label: 'r5.large / Linux', attributes: { instance_type: 'r5.large' } }),
      ],
      instanceFilter: 'r5',
    });

    expect(screen.getByText('r5.large / Linux')).toBeInTheDocument();
    expect(screen.queryByText('m5.large / Linux')).not.toBeInTheDocument();
  });

  it('絞り込みでヒットしない場合は非該当メッセージを表示する', () => {
    renderGroup({ rates: [rate()], instanceFilter: 'no-such-instance' });
    expect(screen.getByText('この条件に一致する単価がありません')).toBeInTheDocument();
  });

  it('hideTitle が true の場合は group 見出しを表示しない (SP カード)', () => {
    renderGroup({
      group: 'Compute Savings Plans',
      rates: [
        rate({
          rateId: 'sp-1',
          model: 'savings_plan',
          group: 'Compute Savings Plans',
          attributes: { instance_type: 'm5.large' },
        }),
      ],
      hideTitle: true,
    });

    expect(screen.queryByText('Compute Savings Plans')).not.toBeInTheDocument();
  });

  it('hideTitle が false/未指定の場合は group 見出しを表示する', () => {
    renderGroup({ group: 'On-Demand', rates: [rate()] });
    expect(screen.getByText('On-Demand')).toBeInTheDocument();
  });

  it('チェックボックス操作で onToggleRate が rateId 付きで呼ばれる', () => {
    const onToggleRate = vi.fn();
    renderGroup({ rates: [rate({ rateId: 'sku.1' })], onToggleRate });

    fireEvent.click(screen.getByRole('checkbox'));
    expect(onToggleRate).toHaveBeenCalledWith('sku.1');
  });

  it('折りたたみボタンで表と条件セレクタを非表示にでき、再クリックで元に戻る', () => {
    const rates = [
      rate({
        rateId: 'std-1yr-noupfront',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'm5.large standard 1yr No Upfront',
        term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
      rate({
        rateId: 'std-3yr-noupfront',
        model: 'reserved',
        group: 'Reserved Instance',
        label: 'm5.large standard 3yr No Upfront',
        term: { lease: '3yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
    ];
    renderGroup({ group: 'Reserved Instance', rates });

    expect(screen.getByText('m5.large standard 1yr No Upfront')).toBeInTheDocument();
    expect(document.querySelector('.pr-group-conditions')).not.toBeNull();

    fireEvent.click(screen.getByTitle('折りたたむ'));

    expect(screen.queryByText('m5.large standard 1yr No Upfront')).not.toBeInTheDocument();
    expect(document.querySelector('.pr-group-conditions')).toBeNull();
    expect(document.querySelector('.pr-rate-table')).toBeNull();

    fireEvent.click(screen.getByTitle('展開'));

    expect(screen.getByText('m5.large standard 1yr No Upfront')).toBeInTheDocument();
    expect(document.querySelector('.pr-group-conditions')).not.toBeNull();
  });

  it('絞り込みで非該当メッセージが出ている状態でも、折りたたむとメッセージごと隠れる', () => {
    renderGroup({ rates: [rate()], instanceFilter: 'no-such-instance' });
    expect(screen.getByText('この条件に一致する単価がありません')).toBeInTheDocument();

    fireEvent.click(screen.getByTitle('折りたたむ'));

    expect(screen.queryByText('この条件に一致する単価がありません')).not.toBeInTheDocument();
  });

  it('折りたたみボタンは aria-expanded で開閉状態を表す', () => {
    renderGroup();
    const btn = screen.getByTitle('折りたたむ');
    expect(btn).toHaveAttribute('aria-expanded', 'true');

    fireEvent.click(btn);
    expect(screen.getByTitle('展開')).toHaveAttribute('aria-expanded', 'false');
  });
});
