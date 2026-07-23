import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { DrawerValueEditor } from './DrawerValueEditor';

const noopSave = () => Promise.resolve();
const noop = () => {};

function renderEditor(overrides: Partial<Parameters<typeof DrawerValueEditor>[0]> = {}) {
  return render(
    <DrawerValueEditor
      infoRows={[['Name', '/app/db']]}
      value="current-value"
      isLoading={false}
      error={null}
      confirmName="/app/db"
      onSave={noopSave}
      onClose={noop}
      {...overrides}
    />,
  );
}

// プレビューから「編集」を押して編集モードに入り、textarea を返す。
function enterEdit(container: HTMLElement): HTMLTextAreaElement {
  fireEvent.click(within(container).getByRole('button', { name: '編集' }));
  return container.querySelector('textarea') as HTMLTextAreaElement;
}

describe('DrawerValueEditor', () => {
  it('プレビューで現在値と参考属性を表示し、初期は textarea を出さない', () => {
    const { container } = renderEditor();
    expect(container.querySelector('textarea')).toBeNull();
    expect(container.textContent).toContain('current-value');
    expect(container.textContent).toContain('/app/db');
  });

  it('「編集」を押すと編集モードに切り替わり現在値を textarea に読み込む', () => {
    const { container } = renderEditor();
    const textarea = enterEdit(container);
    expect(textarea.value).toBe('current-value');
  });

  it('ローディング中は textarea も「編集」ボタンも出さない', () => {
    const { container } = renderEditor({ value: undefined, isLoading: true });
    expect(container.querySelector('textarea')).toBeNull();
    expect(within(container).queryByRole('button', { name: '編集' })).toBeNull();
  });

  it('取得エラー時はエラーメッセージを表示し textarea を出さない', () => {
    const { container } = renderEditor({
      value: undefined,
      error: new Error('sso token expired'),
    });
    expect(container.querySelector('textarea')).toBeNull();
    expect(container.textContent).toContain('sso token expired');
  });

  it('Close ボタンで onClose を呼ぶ', () => {
    const onClose = vi.fn();
    const view = within(renderEditor({ onClose }).container);
    fireEvent.click(view.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('上書き確認をキャンセルすると onSave を呼ばない', () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const { container } = renderEditor({ onSave });
    const view = within(container);
    const textarea = enterEdit(container);
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    fireEvent.click(view.getByRole('button', { name: '保存' }));
    expect(confirmSpy).toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('上書き確認を承認すると編集後の値で onSave を呼び、プレビューに戻る', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container } = renderEditor({ onSave });
    const view = within(container);
    const textarea = enterEdit(container);
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    fireEvent.click(view.getByRole('button', { name: '保存' }));
    expect(onSave).toHaveBeenCalledWith('new-value');
    // 保存成功で編集を閉じ、プレビュー (textarea なし) に戻る。
    await waitFor(() => expect(container.querySelector('textarea')).toBeNull());
    confirmSpy.mockRestore();
  });

  it('保存に失敗した場合はエラーを表示する', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('permission denied'));
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container } = renderEditor({ onSave });
    const view = within(container);
    const textarea = enterEdit(container);
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    fireEvent.click(view.getByRole('button', { name: '保存' }));
    expect(await view.findByText(/permission denied/)).not.toBeNull();
    confirmSpy.mockRestore();
  });
});
