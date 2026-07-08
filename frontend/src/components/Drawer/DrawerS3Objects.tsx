// S3 バケットのオブジェクト一覧・アップロード・ダウンロードを扱う Drawer タブ
import { useMemo, useRef, useState } from 'react';
import { s3DownloadUrl } from '../../api/endpoints';
import { useS3Objects, useS3Upload } from '../../api/queries';
import { s3ObjectColumns, type ColumnDef } from '../tables/columns';
import { DataTable } from '../DataTable';
import type { S3ObjectRow } from '../../types/aws';
import { ApiError } from '../../types/common';

export interface DrawerS3ObjectsProps {
  profile: string;
  region: string;
  bucket: string;
}

// DataTable が要求する id/state を key から射影した行型 (S3 オブジェクトに state はないため空文字)
type S3ObjectTableRow = S3ObjectRow & { id: string; state: string };

export function DrawerS3Objects({ profile, region, bucket }: DrawerS3ObjectsProps) {
  const { data, isLoading, error } = useS3Objects(profile, region, bucket);
  const upload = useS3Upload(profile, region, bucket);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState<File | null>(null);

  const rows = useMemo<S3ObjectTableRow[]>(
    () => (data ?? []).map((r) => ({ ...r, id: r.key, state: '' })),
    [data],
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

  return (
    <div className="section">
      <h3>Objects ({rows.length})</h3>

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <input
          ref={fileInputRef}
          type="file"
          onChange={(e) => setSelected(e.target.files?.[0] ?? null)}
          disabled={upload.isPending}
        />
        <button className="btn sm" onClick={onUpload} disabled={!selected || upload.isPending}>
          {upload.isPending ? 'Uploading…' : 'Upload'}
        </button>
        {upload.error && (
          <span style={{ color: 'var(--err)' }}>
            {upload.error instanceof ApiError ? upload.error.message : String(upload.error)}
          </span>
        )}
      </div>

      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
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
