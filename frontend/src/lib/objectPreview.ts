// S3 / GCS オブジェクトプレビューの活性判定。backend (handlers_object_preview.go) と
// 同じ基準 (拡張子 csv/txt/json、5 MB 未満) をフロント側の行アクション活性判定に使う。
// backend の最終判断はサーバ側で行われるため、ここでの判定はあくまで UI 上のヒント。

// PREVIEW_MAX_SIZE は backend の maxPreviewSize (5 << 20) と同じ値を保つこと。
export const PREVIEW_MAX_SIZE = 5 * 1024 * 1024;

const PREVIEW_ALLOWED_EXTENSIONS = new Set(['.csv', '.txt', '.json']);

// fileExtension は Go の path/filepath.Ext と同じ規則で拡張子 (先頭の "." を含む) を取り出す。
// パス区切り "/" より手前で最初に見つかった "." 以降を返す。区切りが先に見つかったら
// 拡張子なしとして空文字を返す。
export function fileExtension(key: string): string {
  for (let i = key.length - 1; i >= 0; i--) {
    const ch = key[i];
    if (ch === '/') break;
    if (ch === '.') return key.slice(i).toLowerCase();
  }
  return '';
}

// isPreviewEligible はキーとサイズからプレビュー可能かを判定する。
export function isPreviewEligible(key: string, size: number): boolean {
  return PREVIEW_ALLOWED_EXTENSIONS.has(fileExtension(key)) && size < PREVIEW_MAX_SIZE;
}

// previewDisabledReason はプレビュー不可の理由文言を返す (可能な場合は空文字)。
export function previewDisabledReason(key: string, size: number): string {
  if (!PREVIEW_ALLOWED_EXTENSIONS.has(fileExtension(key))) {
    return 'csv / txt / json のみプレビューできます';
  }
  if (size >= PREVIEW_MAX_SIZE) {
    return '5 MB 以上のオブジェクトはプレビューできません';
  }
  return '';
}
