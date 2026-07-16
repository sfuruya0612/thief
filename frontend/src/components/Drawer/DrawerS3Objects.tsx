// S3 バケットのオブジェクト一覧・アップロード・ダウンロードを扱う Drawer タブ。
// 描画本体は DrawerObjectBrowser に共通化されており、ここでは S3 固有の
// フック・キー射影・ダウンロード URL のみを注入する。
import { useCallback } from 'react';
import { s3DownloadUrl } from '../../api/endpoints';
import { useS3Objects, useS3Upload } from '../../api/queries';
import { s3ObjectColumns, type ColumnDef } from '../tables/columns';
import { DrawerObjectBrowser } from './DrawerObjectBrowser';
import type { S3ObjectRow } from '../../types/aws';

export interface DrawerS3ObjectsProps {
  profile: string;
  region: string;
  bucket: string;
}

// DataTable が要求する id/state を key から射影した行型 (S3 オブジェクトに state はないため空文字)
type S3ObjectTableRow = S3ObjectRow & { id: string; state: string };

const keyOf = (r: S3ObjectRow): string => r.key;
const toTableRow = (r: S3ObjectRow): S3ObjectTableRow => ({ ...r, id: r.key, state: '' });
const baseColumns = s3ObjectColumns as ColumnDef<S3ObjectTableRow>[];

export function DrawerS3Objects({ profile, region, bucket }: DrawerS3ObjectsProps) {
  const { data, isLoading, error } = useS3Objects(profile, region, bucket);
  const useUpload = (uploadPrefix: string | undefined) =>
    useS3Upload(profile, region, bucket, uploadPrefix);
  const downloadHref = useCallback(
    (r: S3ObjectTableRow) => s3DownloadUrl(profile, region, bucket, r.key),
    [profile, region, bucket],
  );

  return (
    <DrawerObjectBrowser
      data={data}
      isLoading={isLoading}
      error={error}
      keyOf={keyOf}
      toTableRow={toTableRow}
      baseColumns={baseColumns}
      downloadHref={downloadHref}
      useUpload={useUpload}
    />
  );
}
