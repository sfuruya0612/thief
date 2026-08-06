import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { FetchFailedWarning } from './FetchFailedWarning';

describe('FetchFailedWarning', () => {
  it('警告アイコンと日本語のツールチップを表示する', () => {
    const { container } = render(<FetchFailedWarning />);
    const el = container.querySelector('.fetch-failed-warning');
    expect(el).not.toBeNull();
    expect(el!.getAttribute('title')).toBe(
      '取得に失敗しました (権限不足または一時的なエラーの可能性があります)',
    );
  });
});
