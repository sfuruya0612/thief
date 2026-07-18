import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { AttributeFilterBar } from './AttributeFilterBar';
import type { AttributeFilterSpec } from '../../lib/pricingAttributeFilters';

const RDS_SPECS: AttributeFilterSpec[] = [
  { key: 'engine', label: 'Engine' },
  { key: 'deployment_option', label: 'Deployment' },
];

describe('AttributeFilterBar', () => {
  it('値が2種類以上ある属性だけをチップとして表示する', () => {
    render(
      <AttributeFilterBar
        specs={RDS_SPECS}
        options={{ engine: ['MySQL', 'PostgreSQL'], deployment_option: ['Single-AZ'] }}
        selected={{}}
        onToggle={() => {}}
      />,
    );
    expect(screen.getByText('Engine')).toBeInTheDocument();
    expect(screen.getByText('MySQL')).toBeInTheDocument();
    expect(screen.getByText('PostgreSQL')).toBeInTheDocument();
    // deployment_option は値が1種類のため絞り込む意味がなく非表示
    expect(screen.queryByText('Deployment')).not.toBeInTheDocument();
    expect(screen.queryByText('Single-AZ')).not.toBeInTheDocument();
  });

  it('全属性が1種類以下なら何もレンダリングしない', () => {
    const { container } = render(
      <AttributeFilterBar
        specs={RDS_SPECS}
        options={{ engine: ['MySQL'], deployment_option: ['Single-AZ'] }}
        selected={{}}
        onToggle={() => {}}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('チップクリックで onToggle が (key, value) 付きで呼ばれる', () => {
    const onToggle = vi.fn();
    render(
      <AttributeFilterBar
        specs={RDS_SPECS}
        options={{ engine: ['MySQL', 'PostgreSQL'], deployment_option: [] }}
        selected={{}}
        onToggle={onToggle}
      />,
    );
    fireEvent.click(screen.getByText('PostgreSQL'));
    expect(onToggle).toHaveBeenCalledWith('engine', 'PostgreSQL');
  });

  it('選択済みの値には active クラスと aria-pressed=true が付く', () => {
    render(
      <AttributeFilterBar
        specs={RDS_SPECS}
        options={{ engine: ['MySQL', 'PostgreSQL'], deployment_option: [] }}
        selected={{ engine: new Set(['MySQL']) }}
        onToggle={() => {}}
      />,
    );
    expect(screen.getByText('MySQL')).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('MySQL')).toHaveClass('active');
    expect(screen.getByText('PostgreSQL')).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByText('PostgreSQL')).not.toHaveClass('active');
  });

  it('valueLabels が指定された属性は変換後のラベルで表示し、クリック時は生値を渡す', () => {
    const onToggle = vi.fn();
    const specs: AttributeFilterSpec[] = [
      {
        key: 'storage_type',
        label: 'Storage',
        valueLabels: { standard: 'Standard', io_optimized: 'IO-Optimized' },
      },
    ];
    render(
      <AttributeFilterBar
        specs={specs}
        options={{ storage_type: ['standard', 'io_optimized'] }}
        selected={{}}
        onToggle={onToggle}
      />,
    );
    expect(screen.getByText('Standard')).toBeInTheDocument();
    expect(screen.getByText('IO-Optimized')).toBeInTheDocument();
    fireEvent.click(screen.getByText('IO-Optimized'));
    expect(onToggle).toHaveBeenCalledWith('storage_type', 'io_optimized');
  });
});
