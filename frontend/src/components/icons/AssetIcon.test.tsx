import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { AssetIcon } from './AssetIcon';

// vitest の globals が無効なため RTL の自動 cleanup は効かない。明示的に実行する。
afterEach(cleanup);

describe('AssetIcon', () => {
  it('通常はアセットの img を表示する', () => {
    render(<AssetIcon src="/assets/aws-icons/athena.svg" alt="athena" size={16} />);
    const img = screen.getByRole('img', { name: 'athena' });
    expect(img.tagName).toBe('IMG');
    expect(img).toHaveAttribute('src', '/assets/aws-icons/athena.svg');
  });

  it('読み込み失敗時はプレースホルダへフォールバックする', () => {
    render(<AssetIcon src="/assets/aws-icons/missing.svg" alt="missing" size={16} />);
    fireEvent.error(screen.getByRole('img', { name: 'missing' }));
    const placeholder = screen.getByRole('img', { name: 'missing' });
    expect(placeholder.tagName).toBe('SPAN');
  });
});
