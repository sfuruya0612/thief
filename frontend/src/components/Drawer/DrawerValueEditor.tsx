// Secrets Manager / SSM Parameter Store の値を編集する Drawer の Edit タブ本体 (presentational)。
// 現在値を textarea に読み込み、保存前に上書き確認ダイアログを挟む。値の取得や保存 (API 呼び出し)
// は呼び出し側 (DrawerSecretEdit / DrawerSSMEdit) から props で注入する。
import { useEffect, useState, type ReactNode } from 'react';
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
}: DrawerValueEditorProps) {
  const { t } = useTranslation('drawerAws');
  const [draft, setDraft] = useState('');
  // value が undefined から確定した初回にのみ draft を同期する。以降はユーザーの編集内容を
  // 優先し、バックグラウンド再取得で value が変わっても draft を上書きしない。
  const [initialized, setInitialized] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (!initialized && value !== undefined) {
      setDraft(value);
      setInitialized(true);
    }
  }, [value, initialized]);

  const dirty = initialized && draft !== (value ?? '');

  const handleSave = async () => {
    if (!window.confirm(t('valueEditor.overwriteConfirm', { name: confirmName }))) return;
    setIsSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      await onSave(draft);
      setSaved(true);
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
        <button
          className="btn sm primary"
          style={{ marginLeft: 'auto' }}
          onClick={() => void handleSave()}
          disabled={isSaving || !initialized || !dirty}
        >
          {isSaving ? t('valueEditor.saving') : t('valueEditor.save')}
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

      {isLoading ? (
        <Loading />
      ) : error ? (
        <div style={{ padding: '8px 0', color: 'var(--err)' }}>{errorText(error)}</div>
      ) : (
        <>
          <textarea
            className="object-edit-textarea"
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              setSaved(false);
            }}
            disabled={isSaving || !initialized}
            spellCheck={false}
          />
          {saveError !== null && (
            <div style={{ padding: '8px 0', color: 'var(--err)' }}>{errorText(saveError)}</div>
          )}
          {saved && (
            <div style={{ padding: '8px 0', color: 'var(--ok, var(--text-3))' }}>
              {t('valueEditor.saved')}
            </div>
          )}
        </>
      )}
    </div>
  );
}
