// S3 / GCS オブジェクトプレビューの活性判定。backend (handlers_object_preview.go) と
// 同じ基準 (既知のバイナリ拡張子でなく、かつ 5 MB 未満) をフロント側の行アクション活性判定・
// グレーアウト判定に使う。中身がバイナリかどうかの最終判断はサーバ側 (UTF-8 + NUL 検査) で
// 行われるため、ここでの判定はあくまで一覧時点のヒント (拡張子とサイズのみで決まる)。

// PREVIEW_MAX_SIZE は backend の maxPreviewSize (5 << 20) と同じ値を保つこと。
export const PREVIEW_MAX_SIZE = 5 * 1024 * 1024;

// PREVIEW_BINARY_EXTENSIONS は拡張子だけでバイナリ (プレビュー非対応) と判定できる形式。
// backend の previewBinaryExtensions (handlers_object_preview.go) と同じ集合を保つこと。
const PREVIEW_BINARY_EXTENSIONS = new Set([
  // 画像
  '.png',
  '.jpg',
  '.jpeg',
  '.gif',
  '.bmp',
  '.ico',
  '.webp',
  '.tif',
  '.tiff',
  '.heic',
  '.heif',
  '.avif',
  '.psd',
  // 動画
  '.mp4',
  '.m4v',
  '.mov',
  '.avi',
  '.mkv',
  '.webm',
  '.flv',
  '.wmv',
  '.mpg',
  '.mpeg',
  '.3gp',
  // 音声
  '.mp3',
  '.wav',
  '.flac',
  '.aac',
  '.ogg',
  '.oga',
  '.m4a',
  '.wma',
  '.opus',
  // アーカイブ / 圧縮
  '.zip',
  '.gz',
  '.tgz',
  '.bz2',
  '.tbz2',
  '.xz',
  '.7z',
  '.rar',
  '.zst',
  '.lz4',
  '.lzma',
  '.br',
  // 実行ファイル / バイナリ
  '.exe',
  '.dll',
  '.so',
  '.dylib',
  '.bin',
  '.o',
  '.a',
  '.class',
  '.jar',
  '.war',
  '.wasm',
  '.msi',
  '.deb',
  '.rpm',
  '.apk',
  // バイナリ文書
  '.pdf',
  '.doc',
  '.docx',
  '.xls',
  '.xlsx',
  '.ppt',
  '.pptx',
  '.odt',
  '.ods',
  '.odp',
  // フォント
  '.woff',
  '.woff2',
  '.ttf',
  '.otf',
  '.eot',
  // シリアライズ / データ
  '.parquet',
  '.avro',
  '.orc',
  '.pb',
  '.pyc',
  '.pyo',
  '.npy',
  '.npz',
  '.pkl',
  '.h5',
  '.hdf5',
  '.feather',
  // ディスクイメージ / DB
  '.iso',
  '.dmg',
  '.img',
  '.db',
  '.sqlite',
  '.sqlite3',
  '.mdb',
]);

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

// isBinaryExtension は key の拡張子が既知のバイナリ形式かどうかを判定する。
// 拡張子なし・未知の拡張子は false (テキストの可能性あり) とする。
export function isBinaryExtension(key: string): boolean {
  return PREVIEW_BINARY_EXTENSIONS.has(fileExtension(key));
}

// isPreviewEligible はキーとサイズからプレビュー可能かを判定する。
export function isPreviewEligible(key: string, size: number): boolean {
  return !isBinaryExtension(key) && size < PREVIEW_MAX_SIZE;
}

// previewDisabledReason はプレビュー不可の理由文言を返す (可能な場合は空文字)。
export function previewDisabledReason(key: string, size: number): string {
  if (isBinaryExtension(key)) {
    return 'バイナリファイルはプレビューできません';
  }
  if (size >= PREVIEW_MAX_SIZE) {
    return '5 MB 以上のオブジェクトはプレビューできません';
  }
  return '';
}
