import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { Stat } from './Stat';

describe('Stat', () => {
  it('label と value だけなら .delta を描画しない', () => {
    const { container } = render(<Stat label="Total" value={8} />);
    const stat = container.querySelector('div.stat')!;
    expect(stat.querySelector('.label')!.textContent).toBe('Total');
    expect(stat.querySelector('.value')!.textContent).toBe('8');
    expect(stat.querySelector('.unit')).toBeNull();
    expect(stat.querySelector('.delta')).toBeNull();
  });

  it('unit は value の中の span.unit、delta は tone 付きの div.delta になる', () => {
    const { container } = render(
      <Stat label="Running" value={6} unit="instances" delta="▲ 2" tone="pos" title="hover" />,
    );
    const stat = container.querySelector('div.stat')!;
    expect(stat).toHaveAttribute('title', 'hover');
    expect(stat.querySelector('.value')!.textContent).toBe('6instances');
    expect(stat.querySelector('.value > span.unit')!.textContent).toBe('instances');
    expect(stat.querySelector('.delta')!.className).toBe('delta pos');
    expect(stat.querySelector('.delta')!.textContent).toBe('▲ 2');
  });

  it("高さをそろえるだけの delta (' ') は tone なしの div.delta として描画する", () => {
    const { container } = render(<Stat label="Total" value="$1.00" delta=" " />);
    const delta = container.querySelector('.delta')!;
    expect(delta.className).toBe('delta');
    expect(delta.textContent).toBe(' ');
  });
});
