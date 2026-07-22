import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
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
      onDemandHourlyByLabel={props.onDemandHourlyByLabel}
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

  // issue 0057: RI 行に前払い/月額/実効時間単価/On-Demand 比節減率を表示する。
  describe('Reserved Instance の実効値・節減率表示', () => {
    it('No Upfront: 実効時間単価は継続時間単価と一致し、節減率が算出される', () => {
      renderGroup({
        group: 'Reserved Instance',
        rates: [
          rate({
            rateId: 'ri-no-upfront',
            model: 'reserved',
            group: 'Reserved Instance',
            label: 'm5.large / Linux / Shared',
            priceUSD: 0.08,
            upfrontUSD: 0,
            term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
          }),
        ],
        onDemandHourlyByLabel: new Map([['m5.large / Linux / Shared', 0.1]]),
      });

      expect(screen.getByText('$0.08/時間')).toBeInTheDocument();
      expect(screen.getByText('実効')).toBeInTheDocument();
      expect(screen.getByText('月額 $58.40')).toBeInTheDocument();
      expect(screen.getByText('On-Demand比 20.0%')).toBeInTheDocument();
      // No Upfront は前払いが 0 のため前払い表示は出ない
      expect(screen.queryByText(/前払い \$/)).not.toBeInTheDocument();
    });

    it('All Upfront: 継続時間単価が $0.00 でも実効時間単価は前払いの按分で正しく表示される', () => {
      renderGroup({
        group: 'Reserved Instance',
        rates: [
          rate({
            rateId: 'ri-all-upfront',
            model: 'reserved',
            group: 'Reserved Instance',
            label: 'm5.large / Linux / Shared',
            priceUSD: 0,
            upfrontUSD: 876, // 876 / (730*12) = 0.1
            term: { lease: '1yr', offeringClass: 'standard', payment: 'All Upfront' },
          }),
        ],
        onDemandHourlyByLabel: new Map([['m5.large / Linux / Shared', 0.1]]),
      });

      expect(screen.getByText('$0.10/時間')).toBeInTheDocument(); // 実効時間単価
      expect(screen.getByText('月額 $0.00')).toBeInTheDocument(); // 継続分は 0
      expect(screen.getByText('前払い $876.00')).toBeInTheDocument();
      expect(screen.getByText('On-Demand比 0.0%')).toBeInTheDocument();
    });

    it('Partial Upfront: 継続時間単価と前払いの按分を加算した実効時間単価になる', () => {
      renderGroup({
        group: 'Reserved Instance',
        rates: [
          rate({
            rateId: 'ri-partial-upfront',
            model: 'reserved',
            group: 'Reserved Instance',
            label: 'm5.large / Linux / Shared',
            priceUSD: 0.05,
            upfrontUSD: 438, // 438 / (730*12) = 0.05
            term: { lease: '1yr', offeringClass: 'standard', payment: 'Partial Upfront' },
          }),
        ],
        onDemandHourlyByLabel: new Map([['m5.large / Linux / Shared', 0.1]]),
      });

      expect(screen.getByText('$0.10/時間')).toBeInTheDocument(); // 実効時間単価 (0.05+0.05)
      expect(screen.getByText('前払い $438.00')).toBeInTheDocument();
      expect(screen.getByText('On-Demand比 0.0%')).toBeInTheDocument();
    });

    it('同一 label の On-Demand が見つからない場合、節減率は — になる', () => {
      renderGroup({
        group: 'Reserved Instance',
        rates: [
          rate({
            rateId: 'ri-no-match',
            model: 'reserved',
            group: 'Reserved Instance',
            label: 'm5.large / Linux / Shared',
            priceUSD: 0.08,
            upfrontUSD: 0,
            term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
          }),
        ],
        onDemandHourlyByLabel: new Map(),
      });

      expect(screen.getByText('On-Demand比 —')).toBeInTheDocument();
    });

    it('On-Demand の行には実効値・節減率を表示しない', () => {
      renderGroup({
        group: 'On-Demand',
        rates: [rate({ label: 'm5.large / Linux / Shared', priceUSD: 0.1 })],
        onDemandHourlyByLabel: new Map([['m5.large / Linux / Shared', 0.1]]),
      });

      expect(screen.queryByText('実効')).not.toBeInTheDocument();
      expect(screen.queryByText(/On-Demand比/)).not.toBeInTheDocument();
    });
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

// 手書き windowing の挙動 (hooks/useWindowedRows.ts, lib/windowedRows.ts)。
// jsdom はレイアウトを持たない (getBoundingClientRect / clientHeight / offsetHeight が 0) ため、
// この describe 内でのみ測定値を固定値へ差し替える (外側の describe には影響させない)。
describe('RateGroupSection windowing', () => {
  const proto = HTMLElement.prototype;
  const savedRect = Object.getOwnPropertyDescriptor(proto, 'getBoundingClientRect');
  const savedClientHeight = Object.getOwnPropertyDescriptor(proto, 'clientHeight');
  const savedOffsetHeight = Object.getOwnPropertyDescriptor(proto, 'offsetHeight');

  const VIEWPORT_HEIGHT = 200;
  const ROW_HEIGHT = 20;

  beforeEach(() => {
    Object.defineProperty(proto, 'getBoundingClientRect', {
      configurable: true,
      value: () =>
        ({
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          width: 0,
          height: 0,
          x: 0,
          y: 0,
          toJSON() {},
        }) as DOMRect,
    });
    Object.defineProperty(proto, 'clientHeight', {
      configurable: true,
      get: () => VIEWPORT_HEIGHT,
    });
    Object.defineProperty(proto, 'offsetHeight', { configurable: true, get: () => ROW_HEIGHT });
  });

  afterEach(() => {
    if (savedRect) Object.defineProperty(proto, 'getBoundingClientRect', savedRect);
    if (savedClientHeight) Object.defineProperty(proto, 'clientHeight', savedClientHeight);
    if (savedOffsetHeight) Object.defineProperty(proto, 'offsetHeight', savedOffsetHeight);
  });

  // findScrollParent が検出できるよう overflow-y: auto のスクロール祖先で包む。
  function renderInScroll(ui: ReactElement) {
    return render(<div style={{ overflowY: 'auto' }}>{ui}</div>);
  }
  function dataRows(c: HTMLElement): HTMLElement[] {
    return Array.from(c.querySelectorAll('tbody tr:not(.pr-rate-spacer)'));
  }
  function spacerRows(c: HTMLElement): HTMLElement[] {
    return Array.from(c.querySelectorAll('tbody tr.pr-rate-spacer'));
  }
  function manyRates(n: number): PriceRateRow[] {
    return Array.from({ length: n }, (_, i) =>
      rate({
        rateId: `r${i}`,
        label: `m5.${i}xlarge`,
        attributes: { instance_type: `m5.${i}xlarge` },
      }),
    );
  }

  it('しきい値未満の行数では全行を描画し、スペーサーを出さない', () => {
    const { container } = renderInScroll(
      <RateGroupSection
        group="On-Demand"
        rates={manyRates(5)}
        selection={{}}
        onToggleRate={() => {}}
        instanceFilter=""
      />,
    );
    expect(dataRows(container)).toHaveLength(5);
    expect(spacerRows(container)).toHaveLength(0);
  });

  it('しきい値以上の行数では可視範囲だけを描画し、末尾にスペーサーを挿入する', () => {
    const rates = manyRates(200);
    const { container } = renderInScroll(
      <RateGroupSection
        group="On-Demand"
        rates={rates}
        selection={{}}
        onToggleRate={() => {}}
        instanceFilter=""
      />,
    );
    const visible = dataRows(container);
    // ビューポート 200px / 行高 20px = 10 行 + overscan。全 200 行よりはるかに少ない。
    expect(visible.length).toBeGreaterThan(0);
    expect(visible.length).toBeLessThan(40);
    expect(visible.length).toBeLessThan(rates.length);
    // scrollTop=0 では topPad=0 なので末尾スペーサーのみ存在し、その高さは正 (残り行分)。
    const spacers = spacerRows(container);
    expect(spacers.length).toBeGreaterThanOrEqual(1);
    const bottomSpacerTd = spacers[spacers.length - 1].querySelector('td') as HTMLElement;
    expect(parseFloat(bottomSpacerTd.style.height)).toBeGreaterThan(0);
  });

  it('windowing 中でも先頭行のチェック操作は正しい rateId で通知される', () => {
    const onToggle = vi.fn();
    const { container } = renderInScroll(
      <RateGroupSection
        group="On-Demand"
        rates={manyRates(200)}
        selection={{}}
        onToggleRate={onToggle}
        instanceFilter=""
      />,
    );
    const firstCheckbox = container.querySelector(
      'tbody tr:not(.pr-rate-spacer) input[type=checkbox]',
    ) as HTMLInputElement;
    fireEvent.click(firstCheckbox);
    expect(onToggle).toHaveBeenCalledWith('r0');
  });

  it('インスタンスフィルタで行数がしきい値未満に絞られると全描画に戻る', () => {
    const { container } = renderInScroll(
      <RateGroupSection
        group="On-Demand"
        rates={manyRates(200)}
        selection={{}}
        onToggleRate={() => {}}
        instanceFilter="m5.7xlarge"
      />,
    );
    expect(dataRows(container)).toHaveLength(1);
    expect(spacerRows(container)).toHaveLength(0);
  });
});
