// S3 / GCS オブジェクトブラウザ (DrawerObjectBrowser) から開くプレビュー本体。
// txt/json はテキスト表示 (json は整形)、csv はテーブル表示する。
// 編集モードでは表示形式に関わらず生テキストを直接編集し、保存前に上書き確認を挟む。
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Loading } from '../Loading';
import { ResultTable } from '../query/ResultTable';
import { ApiError } from '../../types/common';
import { fileExtension } from '../../lib/objectPreview';
import { parseCsv } from '../../lib/parseCsv';

export interface DrawerObjectPreviewProps {
  fileName: string;
  content: string | undefined;
  isLoading: boolean;
  error: unknown;
  onClose: () => void;
  // 保存経路 (アップロード API 呼び出し) を呼び出し側から注入する。失敗時は reject する。
  onSave: (content: string) => Promise<void>;
}

function PreviewBody({ fileName, content }: { fileName: string; content: string }) {
  const ext = fileExtension(fileName);

  if (ext === '.csv') {
    const rows = parseCsv(content);
    const [header, ...body] = rows;
    return <ResultTable columns={header ?? []} rows={body} />;
  }

  if (ext === '.json') {
    let formatted = content;
    try {
      formatted = JSON.stringify(JSON.parse(content), null, 2);
    } catch {
      // パースできない JSON は生テキストのまま表示する
    }
    return (
      <pre className="logbox" style={{ maxHeight: 'none' }}>
        {formatted}
      </pre>
    );
  }

  return (
    <pre className="logbox" style={{ maxHeight: 'none' }}>
      {content}
    </pre>
  );
}

export function DrawerObjectPreview({
  fileName,
  content,
  isLoading,
  error,
  onClose,
  onSave,
}: DrawerObjectPreviewProps) {
  const { t } = useTranslation('drawerStorage');
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);

  const startEdit = () => {
    setDraft(content ?? '');
    setSaveError(null);
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setSaveError(null);
  };

  const handleSave = async () => {
    if (!window.confirm(t('drawerObjectPreview.overwriteConfirm', { fileName }))) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await onSave(draft);
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
        <h3 style={{ margin: 0 }}>Preview: {fileName}</h3>
        {!editing && content !== undefined && (
          <button className="btn sm" style={{ marginLeft: 'auto' }} onClick={startEdit}>
            {t('drawerObjectPreview.edit')}
          </button>
        )}
        {editing && (
          <span style={{ display: 'flex', gap: 8, marginLeft: 'auto' }}>
            <button className="btn sm" onClick={cancelEdit} disabled={isSaving}>
              {t('drawerObjectPreview.cancel')}
            </button>
            <button
              className="btn sm primary"
              onClick={() => void handleSave()}
              disabled={isSaving}
            >
              {isSaving ? t('drawerObjectPreview.saving') : t('drawerObjectPreview.save')}
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
      {saveError !== null && (
        <div style={{ padding: '8px 0', color: 'var(--err)' }}>
          {saveError instanceof ApiError ? saveError.message : String(saveError)}
        </div>
      )}
      {editing ? (
        <textarea
          className="object-edit-textarea"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          disabled={isSaving}
          spellCheck={false}
        />
      ) : isLoading ? (
        <Loading />
      ) : error ? (
        <div style={{ padding: 20, color: 'var(--err)' }}>
          {error instanceof ApiError ? error.message : String(error)}
        </div>
      ) : content !== undefined ? (
        <PreviewBody fileName={fileName} content={content} />
      ) : null}
    </div>
  );
}
