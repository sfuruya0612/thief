import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, within } from '@testing-library/react';
import { DrawerObjectPreview } from './DrawerObjectPreview';

const noopSave = () => Promise.resolve();

describe('DrawerObjectPreview', () => {
  it('json は 2 スペースインデントで整形表示する', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="data.json"
        content={'{"a":1,"b":[2,3]}'}
        isLoading={false}
        error={null}
        onClose={() => {}}
        onSave={noopSave}
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
        onSave={noopSave}
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
        onSave={noopSave}
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
        onSave={noopSave}
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
        onSave={noopSave}
      />,
    );
    expect(container.querySelector('pre')).toBeNull();
  });

  it('編集ボタン押下でテキストエリアに切り替わり、内容を編集できる', () => {
    const { container } = render(
      <DrawerObjectPreview
        fileName="notes.txt"
        content={'line1\nline2'}
        isLoading={false}
        error={null}
        onClose={() => {}}
        onSave={noopSave}
      />,
    );
    const view = within(container);
    fireEvent.click(view.getByText('編集'));
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    expect(textarea.value).toBe('line1\nline2');
    fireEvent.change(textarea, { target: { value: 'edited' } });
    expect(textarea.value).toBe('edited');
  });

  it('保存は上書き確認ダイアログで承認されたときのみ onSave を呼ぶ', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const { container } = render(
      <DrawerObjectPreview
        fileName="notes.txt"
        content={'line1'}
        isLoading={false}
        error={null}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    const view = within(container);
    fireEvent.click(view.getByText('編集'));
    fireEvent.click(view.getByText('保存'));
    expect(confirmSpy).toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('確認ダイアログを承認すると onSave を呼び編集モードを終了する', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container } = render(
      <DrawerObjectPreview
        fileName="notes.txt"
        content={'line1'}
        isLoading={false}
        error={null}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    const view = within(container);
    fireEvent.click(view.getByText('編集'));
    const textarea = view.getByDisplayValue('line1');
    fireEvent.change(textarea, { target: { value: 'edited' } });
    await fireEvent.click(view.getByText('保存'));
    expect(onSave).toHaveBeenCalledWith('edited');
    confirmSpy.mockRestore();
  });

  it('保存に失敗した場合はエラーを表示し編集モードを維持する', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('permission denied'));
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container } = render(
      <DrawerObjectPreview
        fileName="notes.txt"
        content={'line1'}
        isLoading={false}
        error={null}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    const view = within(container);
    fireEvent.click(view.getByText('編集'));
    await fireEvent.click(view.getByText('保存'));
    expect(await view.findByText(/permission denied/)).not.toBeNull();
    expect(view.getByDisplayValue('line1')).not.toBeNull();
    confirmSpy.mockRestore();
  });
});
