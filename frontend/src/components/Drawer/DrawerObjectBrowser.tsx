// オブジェクトストレージ (S3 / GCS) の Drawer タブ共通実装。オブジェクト一覧 + prefix
// フィルタ + アップロード + ダウンロードリンクをまとめて描画する。ストレージ差分
// (取得結果・アップロードフック・キー項目・列定義・ダウンロード URL) は props で注入する。
import { useMemo, useRef, useState } from 'react';
import type { UseMutationResult } from '@tanstack/react-query';
import type { ColumnDef } from '../tables/columns';
import { DataTable } from '../DataTable';
import { Loading } from '../Loading';
import { ApiError } from '../../types/common';

// stripLeadingSlashes は一覧の前方一致フィルタに使う prefix の先頭スラッシュのみを
// 取り除く。末尾は加工しない (入力途中の "log" でも "logs/..." に前方一致させるため)。
function stripLeadingSlashes(prefix: string): string {
  return prefix.replace(/^\/+/, '');
}

// normalizeUploadPrefix はアップロード先フォルダを確定するための prefix を正規化する。
// 先頭・末尾のスラッシュを取り除き、空でなければ末尾にちょうど 1 つのスラッシュを付ける。
function normalizeUploadPrefix(prefix: string): string {
  const trimmed = prefix.trim().replace(/^\/+/, '').replace(/\/+$/, '');
  return trimmed ? `${trimmed}/` : '';
}

// ObjectUploadMutation は useS3Upload / useGcsUpload の戻り値の形。
export type ObjectUploadMutation = UseMutationResult<
  { status: string; key: string },
  Error,
  { key: string; file: File },
  unknown
>;

export interface DrawerObjectBrowserProps<TObject, TRow extends { id: string }> {
  data: TObject[] | undefined;
  isLoading: boolean;
  error: unknown;
  // keyOf は prefix 前方一致フィルタに使うオブジェクトキーを取り出す。
  keyOf: (obj: TObject) => string;
  // toTableRow は DataTable が要求する id/state を持つ行へ射影する。
  toTableRow: (obj: TObject) => TRow;
  baseColumns: ColumnDef<TRow>[];
  downloadHref: (row: TRow) => string;
  // useUpload はアップロード先 prefix (内部 state 由来) を受け取るカスタムフック。
  // コンポーネント描画ごとに必ず 1 回呼ばれる。
  useUpload: (uploadPrefix: string | undefined) => ObjectUploadMutation;
}

export function DrawerObjectBrowser<TObject, TRow extends { id: string }>({
  data,
  isLoading,
  error,
  keyOf,
  toTableRow,
  baseColumns,
  downloadHref,
  useUpload,
}: DrawerObjectBrowserProps<TObject, TRow>) {
  const [prefixInput, setPrefixInput] = useState('');
  const filterPrefix = stripLeadingSlashes(prefixInput);
  const uploadPrefix = normalizeUploadPrefix(prefixInput);
  const upload = useUpload(uploadPrefix || undefined);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);

  // 一覧は常に全件取得済みのものを受け取り、prefix フィルタはフロントエンド側で行う
  // (入力の都度 API を再実行しないようにするため)。
  const rows = useMemo<TRow[]>(
    () => (data ?? []).filter((o) => keyOf(o).startsWith(filterPrefix)).map(toTableRow),
    [data, filterPrefix, keyOf, toTableRow],
  );

  // 共通列にダウンロードリンクの Actions 列を末尾に追加する
  const columns = useMemo<ColumnDef<TRow>[]>(
    () => [
      ...baseColumns,
      {
        key: 'actions',
        header: '',
        width: '10%',
        cell: (r) => (
          <a href={downloadHref(r)} download className="btn sm" style={{ padding: '2px 8px' }}>
            Download
          </a>
        ),
      },
    ],
    [baseColumns, downloadHref],
  );

  const onUpload = () => {
    if (!selected) return;
    upload.mutate(
      { key: selected.name, file: selected },
      {
        onSuccess: () => {
          setSelected(null);
          if (fileInputRef.current) fileInputRef.current.value = '';
        },
      },
    );
  };

  const pickFile = (file: File | null) => {
    if (upload.isPending) return;
    setSelected(file);
  };

  return (
    <div className="section">
      <h3>Objects ({rows.length})</h3>

      <span className="chip-search s3-prefix-input">
        <input
          placeholder="prefix (folder/subfolder)…"
          value={prefixInput}
          onChange={(e) => setPrefixInput(e.target.value)}
        />
      </span>

      <div className="s3-upload">
        <label
          className={`s3-upload-dropzone ${dragOver ? 'drag-over' : ''}`}
          onDragOver={(e) => {
            e.preventDefault();
            if (!upload.isPending) setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragOver(false);
            pickFile(e.dataTransfer.files?.[0] ?? null);
          }}
        >
          <input
            ref={fileInputRef}
            type="file"
            className="s3-upload-input"
            onChange={(e) => pickFile(e.target.files?.[0] ?? null)}
            disabled={upload.isPending}
          />
          <span className="s3-upload-text">
            {selected ? selected.name : 'ファイルを選択またはドロップ'}
          </span>
        </label>
        <button
          className="btn sm primary"
          onClick={onUpload}
          disabled={!selected || upload.isPending}
        >
          {upload.isPending ? 'Uploading…' : 'Upload'}
        </button>
        {upload.error && (
          <span style={{ color: 'var(--err)' }}>
            {upload.error instanceof ApiError ? upload.error.message : String(upload.error)}
          </span>
        )}
      </div>

      {isLoading ? (
        <Loading />
      ) : error ? (
        <div style={{ padding: 20, color: 'var(--err)' }}>
          {error instanceof ApiError ? error.message : String(error)}
        </div>
      ) : (
        <DataTable rows={rows} columns={columns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}
