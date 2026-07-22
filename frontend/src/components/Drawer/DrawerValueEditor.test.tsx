import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, within } from '@testing-library/react';
import { DrawerValueEditor } from './DrawerValueEditor';

const noopSave = () => Promise.resolve();

function renderEditor(overrides: Partial<Parameters<typeof DrawerValueEditor>[0]> = {}) {
  return render(
    <DrawerValueEditor
      infoRows={[['Name', '/app/db']]}
      value="current-value"
      isLoading={false}
      error={null}
      confirmName="/app/db"
      onSave={noopSave}
      {...overrides}
    />,
  );
}

describe('DrawerValueEditor', () => {
  it('現在値を textarea に読み込み、参考属性を表示する', () => {
    const { container } = renderEditor();
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    expect(textarea.value).toBe('current-value');
    expect(container.textContent).toContain('/app/db');
  });

  it('未変更のときは保存ボタンを無効化する', () => {
    const view = within(renderEditor().container);
    expect(view.getByRole('button', { name: '保存' })).toBeDisabled();
  });

  it('値を変更すると保存ボタンが有効になる', () => {
    const { container } = renderEditor();
    const view = within(container);
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    expect(view.getByRole('button', { name: '保存' })).toBeEnabled();
  });

  it('ローディング中は textarea を表示しない', () => {
    const { container } = renderEditor({ value: undefined, isLoading: true });
    expect(container.querySelector('textarea')).toBeNull();
  });

  it('取得エラー時はエラーメッセージを表示し textarea を出さない', () => {
    const { container } = renderEditor({
      value: undefined,
      error: new Error('sso token expired'),
    });
    expect(container.querySelector('textarea')).toBeNull();
    expect(container.textContent).toContain('sso token expired');
  });

  it('現在値を取得できていない場合は保存ボタンを無効のままにする', () => {
    // value が undefined (行が見つからない等) のときは盲目的な上書きを防ぐため保存不可。
    const view = within(renderEditor({ value: undefined }).container);
    expect(view.getByRole('button', { name: '保存' })).toBeDisabled();
  });

  it('上書き確認をキャンセルすると onSave を呼ばない', () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const { container } = renderEditor({ onSave });
    const view = within(container);
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    fireEvent.click(view.getByRole('button', { name: '保存' }));
    expect(confirmSpy).toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('上書き確認を承認すると編集後の値で onSave を呼び、保存完了を表示する', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container } = renderEditor({ onSave });
    const view = within(container);
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    await fireEvent.click(view.getByRole('button', { name: '保存' }));
    expect(onSave).toHaveBeenCalledWith('new-value');
    expect(await view.findByText('保存しました')).not.toBeNull();
    confirmSpy.mockRestore();
  });

  it('保存に失敗した場合はエラーを表示する', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('permission denied'));
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container } = renderEditor({ onSave });
    const view = within(container);
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: 'new-value' } });
    await fireEvent.click(view.getByRole('button', { name: '保存' }));
    expect(await view.findByText(/permission denied/)).not.toBeNull();
    confirmSpy.mockRestore();
  });
});
