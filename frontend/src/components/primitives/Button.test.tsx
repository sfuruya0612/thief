import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { createRef } from 'react';
import { Button } from './Button';

// 置き換え前の className 直書き (btn / btn sm / btn sm ghost / btn sm primary / 追加クラス) と
// 同じクラス列になることを固定する (docs/issues/closed/0203)。
describe('Button', () => {
  it.each([
    [{}, 'btn'],
    [{ size: 'sm' } as const, 'btn sm'],
    [{ size: 'sm', variant: 'ghost' } as const, 'btn sm ghost'],
    [{ size: 'sm', variant: 'primary' } as const, 'btn sm primary'],
    [{ variant: 'primary' } as const, 'btn primary'],
    [{ size: 'sm', className: 'lv-copy-btn' } as const, 'btn sm lv-copy-btn'],
    [{ size: 'sm', variant: 'ghost', className: 'clear-btn' } as const, 'btn sm ghost clear-btn'],
  ])('%o は className が %s になる', (props, expected) => {
    render(<Button {...props}>label</Button>);
    expect(screen.getByRole('button', { name: 'label' }).className).toBe(expected);
  });

  it('<button type="button"> を描画し、onClick / disabled / title を通す', () => {
    const onClick = vi.fn();
    render(
      <Button onClick={onClick} title="Refresh" disabled>
        Refresh
      </Button>,
    );
    const el = screen.getByRole('button', { name: 'Refresh' });
    expect(el.tagName).toBe('BUTTON');
    expect(el).toHaveAttribute('type', 'button');
    expect(el).toHaveAttribute('title', 'Refresh');
    expect(el).toBeDisabled();
    fireEvent.click(el);
    expect(onClick).not.toHaveBeenCalled();
  });

  it('type を渡せば上書きでき、ref が <button> に届く', () => {
    const ref = createRef<HTMLButtonElement>();
    render(
      <Button ref={ref} type="submit">
        Save
      </Button>,
    );
    expect(ref.current?.tagName).toBe('BUTTON');
    expect(ref.current).toHaveAttribute('type', 'submit');
  });
});
