// GCS バケットのオブジェクト一覧・アップロード・ダウンロードを扱う Drawer タブ。
// 描画本体は DrawerObjectBrowser に共通化されており、ここでは GCS 固有の
// フック・キー射影・ダウンロード URL のみを注入する。
import { useCallback } from 'react';
import { gcsDownloadUrl } from '../../api/endpoints';
import { useGcsObjects, useGcsUpload } from '../../api/queries';
import { gcsObjectColumns } from '../tables/gcpColumns';
import type { ColumnDef } from '../tables/columns';
import { DrawerObjectBrowser } from './DrawerObjectBrowser';
import type { GcsObjectRow } from '../../types/gcp';

export interface DrawerGCSObjectsProps {
  projectId: string;
  bucket: string;
}

// DataTable が要求する id/state を GcsObjectRow から射影した行型 (GCS オブジェクトに state はないため空文字)
type GcsObjectTableRow = GcsObjectRow & { state: string };

const keyOf = (r: GcsObjectRow): string => r.name;
const toTableRow = (r: GcsObjectRow): GcsObjectTableRow => ({ ...r, state: '' });
const baseColumns = gcsObjectColumns as ColumnDef<GcsObjectTableRow>[];

export function DrawerGCSObjects({ projectId, bucket }: DrawerGCSObjectsProps) {
  const { data, isLoading, error } = useGcsObjects(projectId, bucket);
  const useUpload = (uploadPrefix: string | undefined) =>
    useGcsUpload(projectId, bucket, uploadPrefix);
  const downloadHref = useCallback(
    (r: GcsObjectTableRow) => gcsDownloadUrl(projectId, bucket, r.name),
    [projectId, bucket],
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
