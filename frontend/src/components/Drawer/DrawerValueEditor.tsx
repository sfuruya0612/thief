// Secrets Manager / SSM Parameter Store の値を参照・編集する Drawer の Value タブ本体
// (presentational)。初期はプレビュー (read-only) で、Edit で編集に切り替え、保存前に
// 上書き確認ダイアログを挟む。値の取得や保存 (API 呼び出し) は呼び出し側
// (DrawerSecretEdit / DrawerSSMEdit) から props で注入する。
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Loading } from '../Loading';
import { ApiError } from '../../types/common';

export interface DrawerValueEditorProps {
  // 参考表示する属性 (Name / Type / Description など)。編集対象ではない。
  infoRows: [string, ReactNode][];
  // 現在値。未取得 (ローディング中や取得失敗) のときは undefined。
  value: string | undefined;
  isLoading: boolean;
  error: unknown;
  // 上書き確認ダイアログに表示する対象名。
  confirmName: string;
  // 保存経路 (更新 API 呼び出し)。失敗時は reject する。
  onSave: (value: string) => Promise<void>;
  // Drawer を閉じる。
  onClose: () => void;
}

function errorText(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}

export function DrawerValueEditor({
  infoRows,
  value,
  isLoading,
  error,
  confirmName,
  onSave,
  onClose,
}: DrawerValueEditorProps) {
  const { t } = useTranslation('drawerAws');
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);

  const startEdit = () => {
    setDraft(value ?? '');
    setSaveError(null);
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setSaveError(null);
  };

  const handleSave = async () => {
    if (!window.confirm(t('valueEditor.overwriteConfirm', { name: confirmName }))) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await onSave(draft);
      // 保存成功で編集を閉じ、プレビューに戻る。更新側の invalidate で値が再取得される。
      setEditing(false);
    } catch (e) {
      setSaveError(e);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="section">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}>{t('valueEditor.title')}</h3>
        {!editing && value !== undefined && (
          <button className="btn sm" style={{ marginLeft: 'auto' }} onClick={startEdit}>
            {t('valueEditor.edit')}
          </button>
        )}
        {editing && (
          <span style={{ display: 'flex', gap: 8, marginLeft: 'auto' }}>
            <button className="btn sm" onClick={cancelEdit} disabled={isSaving}>
              {t('valueEditor.cancel')}
            </button>
            <button
              className="btn sm primary"
              onClick={() => void handleSave()}
              disabled={isSaving}
            >
              {isSaving ? t('valueEditor.saving') : t('valueEditor.save')}
            </button>
          </span>
        )}
        <button
          className="btn sm"
          style={editing ? undefined : { marginLeft: 8 }}
          onClick={onClose}
          disabled={isSaving}
        >
          Close
        </button>
      </div>

      <div className="kv" style={{ marginBottom: 12 }}>
        {infoRows.map(([k, v]) => (
          <div key={k} style={{ display: 'contents' }}>
            <div className="k">{k}</div>
            <div className="v">{v}</div>
          </div>
        ))}
      </div>

      {editing ? (
        <>
          <textarea
            className="object-edit-textarea"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            disabled={isSaving}
            spellCheck={false}
          />
          {saveError !== null && (
            <div style={{ padding: '8px 0', color: 'var(--err)' }}>{errorText(saveError)}</div>
          )}
        </>
      ) : isLoading ? (
        <Loading />
      ) : error ? (
        <div style={{ padding: '8px 0', color: 'var(--err)' }}>{errorText(error)}</div>
      ) : value !== undefined ? (
        <pre className="logbox" style={{ maxHeight: 'none' }}>
          {value}
        </pre>
      ) : null}
    </div>
  );
}
