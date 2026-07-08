import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { StatsRow } from './StatsRow';

// stat の label 一覧を DOM から取り出すヘルパー
function labelsOf(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('.stat .label')).map((el) => el.textContent ?? '');
}

// 指定ラベルに対応する value の数値を取り出す
function valueOf(container: HTMLElement, label: string): string {
  const labels = Array.from(container.querySelectorAll('.stat .label'));
  const found = labels.find((el) => el.textContent === label);
  if (!found) throw new Error(`label not found: ${label}`);
  const parent = found.parentElement as HTMLElement;
  const value = parent.querySelector('.value');
  return value?.textContent ?? '';
}

describe('StatsRow', () => {
  it('ecr は Resources のみ表示する', () => {
    const { container } = render(
      <StatsRow service="ecr" resources={[{ state: '' }, { state: '' }]} cost={[]} />,
    );
    expect(labelsOf(container)).toEqual(['Resources']);
    expect(valueOf(container, 'Resources')).toBe('2');
  });

  it('ssm は Resources のみ表示する', () => {
    const { container } = render(<StatsRow service="ssm" resources={[{ state: '' }]} cost={[]} />);
    expect(labelsOf(container)).toEqual(['Resources']);
  });

  it('secrets は Resources のみ表示する', () => {
    const { container } = render(
      <StatsRow service="secrets" resources={[{ state: '' }]} cost={[]} />,
    );
    expect(labelsOf(container)).toEqual(['Resources']);
  });

  it('elb は Resources と cost 2 種のみ表示する (Active/Other を出さない)', () => {
    const { container } = render(
      <StatsRow service="elb" resources={[{ state: 'active' }, { state: 'active' }]} cost={[]} />,
    );
    expect(labelsOf(container)).toEqual([
      'Resources',
      'Monthly cost (Unblended)',
      'Monthly cost (Net Amortized)',
    ]);
  });

  it('cache は Stopped を含まず available を Running にカウントする', () => {
    const { container } = render(
      <StatsRow
        service="cache"
        resources={[{ state: 'available' }, { state: 'available' }, { state: 'creating' }]}
        cost={[]}
      />,
    );
    const labels = labelsOf(container);
    expect(labels).not.toContain('Stopped');
    expect(labels).toContain('Running');
    expect(valueOf(container, 'Running')).toBe('2');
    expect(valueOf(container, 'Other')).toBe('1');
  });

  it('cloudfront は deployed / in-progress を個別カウントし Other が 0 になる', () => {
    const { container } = render(
      <StatsRow
        service="cloudfront"
        resources={[{ state: 'deployed' }, { state: 'deployed' }, { state: 'in-progress' }]}
        cost={[]}
      />,
    );
    expect(valueOf(container, 'Deployed')).toBe('2');
    expect(valueOf(container, 'In Progress')).toBe('1');
    expect(valueOf(container, 'Other')).toBe('0');
  });

  it('ecs は Stopped を表示しない', () => {
    const { container } = render(
      <StatsRow
        service="ecs"
        resources={[{ state: 'active', activeServices: 3, runningTasks: 5, pendingTasks: 1 }]}
        cost={[]}
      />,
    );
    const labels = labelsOf(container);
    expect(labels).not.toContain('Stopped');
    expect(labels).toContain('Desired');
    expect(labels).toContain('Running');
    expect(labels).toContain('Pending');
    expect(valueOf(container, 'Desired')).toBe('3');
    expect(valueOf(container, 'Running')).toBe('5');
    expect(valueOf(container, 'Pending')).toBe('1');
  });

  it('lambda は Active / Inactive をそれぞれカウントする', () => {
    const { container } = render(
      <StatsRow
        service="lambda"
        resources={[
          { state: 'active' },
          { state: 'active' },
          { state: 'inactive' },
          { state: 'failed' },
        ]}
        cost={[]}
      />,
    );
    expect(valueOf(container, 'Active')).toBe('2');
    expect(valueOf(container, 'Inactive')).toBe('1');
    expect(valueOf(container, 'Other')).toBe('1');
  });
});
