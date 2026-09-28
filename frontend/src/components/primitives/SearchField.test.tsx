import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { createRef } from 'react';
import { SearchField } from './SearchField';

describe('SearchField', () => {
  it('span.chip-search の中に検索アイコン (svg) と input を順に描画する', () => {
    const { container } = render(<SearchField placeholder="filter…" />);
    const span = container.querySelector('span.chip-search')!;
    expect(span).not.toBeNull();
    expect(span.children).toHaveLength(2);
    expect(span.children[0].tagName.toLowerCase()).toBe('svg');
    expect(span.children[1].tagName).toBe('INPUT');
    expect(screen.getByPlaceholderText('filter…')).toBe(span.children[1]);
  });

  it('props は input に渡り、ref も input に届く', () => {
    const onChange = vi.fn();
    const onKeyDown = vi.fn();
    const ref = createRef<HTMLInputElement>();
    render(
      <SearchField
        ref={ref}
        aria-label="Query"
        value="abc"
        onChange={onChange}
        onKeyDown={onKeyDown}
        title="Filter"
      />,
    );
    const input = screen.getByLabelText('Query');
    expect(ref.current).toBe(input);
    expect(input).toHaveValue('abc');
    expect(input).toHaveAttribute('title', 'Filter');
    fireEvent.change(input, { target: { value: 'abcd' } });
    expect(onChange).toHaveBeenCalled();
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onKeyDown).toHaveBeenCalled();
  });

  it('className は外側の span に足す', () => {
    const { container } = render(<SearchField className="wide" />);
    expect(container.querySelector('span')!.className).toBe('chip-search wide');
  });
});
