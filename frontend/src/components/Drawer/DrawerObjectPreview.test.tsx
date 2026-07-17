import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { DrawerObjectPreview } from './DrawerObjectPreview';

describe('DrawerObjectPreview', () => {
  it('json は 2 スペースインデントで整形表示する', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="data.json"
        content={'{"a":1,"b":[2,3]}'}
        isLoading={false}
        error={null}
        onClose={() => {}}
      />,
    );
    const pre = container.querySelector('pre');
    expect(pre?.textContent).toBe(JSON.stringify({ a: 1, b: [2, 3] }, null, 2));
  });

  it('パースできない json は生テキストへフォールバックする', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="broken.json"
        content={'{not valid json'}
        isLoading={false}
        error={null}
        onClose={() => {}}
      />,
    );
    const pre = container.querySelector('pre');
    expect(pre?.textContent).toBe('{not valid json');
  });

  it('csv はテーブル表示する (1 行目はヘッダ)', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="data.csv"
        content={'id,name\n1,alice\n2,bob'}
        isLoading={false}
        error={null}
        onClose={() => {}}
      />,
    );
    expect(container.querySelector('table')).not.toBeNull();
    expect(container.textContent).toContain('alice');
    expect(container.textContent).toContain('bob');
  });

  it('txt は等幅テキストでそのまま表示する', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="notes.txt"
        content={'line1\nline2'}
        isLoading={false}
        error={null}
        onClose={() => {}}
      />,
    );
    expect(container.querySelector('pre')?.textContent).toBe('line1\nline2');
  });

  it('ローディング中はテキストを表示しない', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="notes.txt"
        content={undefined}
        isLoading={true}
        error={null}
        onClose={() => {}}
      />,
    );
    expect(container.querySelector('pre')).toBeNull();
  });
});
