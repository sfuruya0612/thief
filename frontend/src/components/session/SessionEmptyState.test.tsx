import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { SessionEmptyState } from './SessionEmptyState';

afterEach(cleanup);

describe('SessionEmptyState', () => {
  it('タイトルと ＋ 導線のヒントを表示する', () => {
    render(
      <SessionEmptyState
        title="プロファイルを開いてください"
        hint="上のタブバーの「＋ プロファイルを追加」から接続するプロファイルを選択します"
      />,
    );
    expect(screen.getByText('プロファイルを開いてください')).toBeInTheDocument();
    expect(screen.getByText(/＋ プロファイルを追加/)).toBeInTheDocument();
  });
});
