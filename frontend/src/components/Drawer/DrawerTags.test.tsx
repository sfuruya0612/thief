import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { DrawerTags } from './DrawerTags';

describe('DrawerTags', () => {
  it('fetchFailed が未指定なら Tags (件数) を見出しに表示する', () => {
    const { container } = render(<DrawerTags tags={{ Env: 'prod', Team: 'core' }} />);
    expect(container.querySelector('h3')?.textContent).toBe('Tags (2)');
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
  });

  it('fetchFailed が true なら取得失敗の見出しと警告アイコンを表示する', () => {
    const { container } = render(<DrawerTags tags={undefined} fetchFailed />);
    expect(container.querySelector('h3')?.textContent).toContain('取得失敗');
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
  });

  it('fetchFailed が false なら通常どおり件数見出しを表示する', () => {
    const { container } = render(<DrawerTags tags={{}} fetchFailed={false} />);
    expect(container.querySelector('h3')?.textContent).toBe('Tags (0)');
  });
});
