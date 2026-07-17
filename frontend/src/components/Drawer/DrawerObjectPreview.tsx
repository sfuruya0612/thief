// S3 / GCS オブジェクトブラウザ (DrawerObjectBrowser) から開くプレビュー本体。
// txt/json はテキスト表示 (json は整形)、csv はテーブル表示する。
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
}: DrawerObjectPreviewProps) {
  return (
    <div className="section">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}>Preview: {fileName}</h3>
        <button className="btn sm" style={{ marginLeft: 'auto' }} onClick={onClose}>
          Close
        </button>
      </div>
      {isLoading ? (
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
