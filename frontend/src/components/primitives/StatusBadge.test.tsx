import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
  it.each([
    ['draining', 'warn'],
    ['deregistering', 'warn'],
    ['registration-failed', 'err'],
    ['registering', 'info'],
    ['active', 'ok'],
  ])('%s は %s クラスで表示する', (state, cls) => {
    const { container } = render(<StatusBadge state={state} />);
    const span = container.querySelector('span.status')!;
    expect(span.className).toBe(`status ${cls}`);
    expect(span.textContent).toBe(state);
  });

  it('MAP に無い状態は muted で state をそのままラベルにする', () => {
    const { container } = render(<StatusBadge state="something-else" />);
    const span = container.querySelector('span.status')!;
    expect(span.className).toBe('status muted');
    expect(span.textContent).toBe('something-else');
  });
});
