// S3 バケットのオブジェクト一覧・アップロード・ダウンロードを扱う Drawer タブ
import { useMemo, useRef, useState } from 'react';
import { s3DownloadUrl } from '../../api/endpoints';
import { useS3Objects, useS3Upload } from '../../api/queries';
import { s3ObjectColumns, type ColumnDef } from '../tables/columns';
import { DataTable } from '../DataTable';
import { Loading } from '../Loading';
import type { S3ObjectRow } from '../../types/aws';
import { ApiError } from '../../types/common';

export interface DrawerS3ObjectsProps {
  profile: string;
  region: string;
  bucket: string;
}

// DataTable が要求する id/state を key から射影した行型 (S3 オブジェクトに state はないため空文字)
type S3ObjectTableRow = S3ObjectRow & { id: string; state: string };

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

export function DrawerS3Objects({ profile, region, bucket }: DrawerS3ObjectsProps) {
  const [prefixInput, setPrefixInput] = useState('');
  const filterPrefix = stripLeadingSlashes(prefixInput);
  const uploadPrefix = normalizeUploadPrefix(prefixInput);
  // 一覧は常に全件取得し、prefix フィルタはフロントエンド側で行う
  // (入力の都度 API を再実行しないようにするため)。
  const { data, isLoading, error } = useS3Objects(profile, region, bucket);
  const upload = useS3Upload(profile, region, bucket, uploadPrefix || undefined);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);

  const rows = useMemo<S3ObjectTableRow[]>(
    () =>
      (data ?? [])
        .filter((r) => r.key.startsWith(filterPrefix))
        .map((r) => ({ ...r, id: r.key, state: '' })),
    [data, filterPrefix],
  );

  // 共通列にダウンロードリンクの Actions 列を末尾に追加する
  const columns = useMemo<ColumnDef<S3ObjectTableRow>[]>(
    () => [
      ...(s3ObjectColumns as ColumnDef<S3ObjectTableRow>[]),
      {
        key: 'actions',
        header: '',
        width: '10%',
        cell: (r) => (
          <a
            href={s3DownloadUrl(profile, region, bucket, r.key)}
            download
            className="btn sm"
            style={{ padding: '2px 8px' }}
          >
            Download
          </a>
        ),
      },
    ],
    [profile, region, bucket],
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
