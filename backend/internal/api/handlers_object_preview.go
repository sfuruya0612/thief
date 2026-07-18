package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxPreviewSize は S3 / GCS オブジェクトプレビューの上限サイズ。TODO の要件
// 「5 MB 以上のものはプレビュー不可」に合わせ、5 MB 未満のみ許可する (5 MB ちょうどは不可)。
const maxPreviewSize = 5 << 20 // 5MiB

// errPreviewTooLarge は読み込んだ body が maxPreviewSize を超えたことを示す。
var errPreviewTooLarge = fmt.Errorf("object exceeds max preview size of %d bytes", maxPreviewSize)

// previewBinaryExtensions は拡張子だけでバイナリ (プレビュー非対応) と判定できる形式
// (大文字小文字を区別しない)。ここに無い拡張子は「テキストの可能性あり」として扱い、
// 最終的な可否は buildPreviewResponse の中身検査 (UTF-8 妥当性 + NUL バイト非混入) が決める。
// frontend の PREVIEW_BINARY_EXTENSIONS (lib/objectPreview.ts) と同じ集合を保つこと。
var previewBinaryExtensions = map[string]bool{
	// 画像
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".webp": true, ".tif": true, ".tiff": true, ".heic": true,
	".heif": true, ".avif": true, ".psd": true,
	// 動画
	".mp4": true, ".m4v": true, ".mov": true, ".avi": true, ".mkv": true,
	".webm": true, ".flv": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".3gp": true,
	// 音声
	".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true,
	".oga": true, ".m4a": true, ".wma": true, ".opus": true,
	// アーカイブ / 圧縮
	".zip": true, ".gz": true, ".tgz": true, ".bz2": true, ".tbz2": true,
	".xz": true, ".7z": true, ".rar": true, ".zst": true, ".lz4": true,
	".lzma": true, ".br": true,
	// 実行ファイル / バイナリ
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".o": true, ".a": true, ".class": true, ".jar": true, ".war": true,
	".wasm": true, ".msi": true, ".deb": true, ".rpm": true, ".apk": true,
	// バイナリ文書
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
	// フォント
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	// シリアライズ / データ
	".parquet": true, ".avro": true, ".orc": true, ".pb": true, ".pyc": true,
	".pyo": true, ".npy": true, ".npz": true, ".pkl": true, ".h5": true,
	".hdf5": true, ".feather": true,
	// ディスクイメージ / DB
	".iso": true, ".dmg": true, ".img": true,
	".db": true, ".sqlite": true, ".sqlite3": true, ".mdb": true,
}

// PreviewResponse は S3 / GCS オブジェクトプレビューの共通レスポンス。
type PreviewResponse struct {
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// previewExtensionAllowed は key の拡張子 (最終セグメントのみ、大文字小文字を区別しない) が
// プレビュー対象になりうるかを判定する。既知のバイナリ拡張子ならプレビュー不可 (false)、
// それ以外 (拡張子なしを含む) はテキストの可能性ありとして true を返す。中身がバイナリな
// テキスト拡張子は buildPreviewResponse の中身検査が最終的に弾く。"file.txt.gz" のような
// 多重拡張子は最後の拡張子 (.gz) のみで判定する。
func previewExtensionAllowed(key string) bool {
	return !previewBinaryExtensions[strings.ToLower(filepath.Ext(key))]
}

// previewSizeAllowed は size (メタデータ由来のオブジェクトサイズ) が maxPreviewSize 未満かを
// 判定する。TODO の要件「5 MB 以上のものはプレビュー不可」に合わせ、ちょうど 5 MB は不可とする。
func previewSizeAllowed(size int64) bool {
	return size < maxPreviewSize
}

func writePreviewUnsupportedType(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "PREVIEW_UNSUPPORTED_TYPE",
		"preview is not supported for binary file types")
}

func writePreviewTooLarge(w http.ResponseWriter) {
	writeError(w, http.StatusRequestEntityTooLarge, "PREVIEW_TOO_LARGE",
		fmt.Sprintf("preview is supported only for objects under %d bytes", maxPreviewSize))
}

func writePreviewNotText(w http.ResponseWriter) {
	writeError(w, http.StatusUnprocessableEntity, "PREVIEW_NOT_TEXT",
		"object content appears to be binary (contains NUL bytes or is not valid UTF-8)")
}

// readPreviewBody は r から最大 maxPreviewSize+1 バイトを読み込み、上限超過を検出する。
// メタデータ (ContentLength 等) が信頼できず実体がそれより大きいケースへの防御であり、
// 上限を超えた場合は errPreviewTooLarge を返す (readS3UploadBody と同じパターン)。
func readPreviewBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxPreviewSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxPreviewSize {
		return nil, errPreviewTooLarge
	}
	return body, nil
}

// buildPreviewResponse は拡張子ガード・サイズ上限を経た body を読み込み、中身がテキストか
// 検査して PreviewResponse を組み立てる。呼び出し側は key の拡張子ガードとサイズ事前判定
// (ContentLength 等) を済ませた上で、body (未読み込みの Reader) と content type を渡すこと。
// テキスト判定は「有効な UTF-8 かつ NUL バイトを含まない」で行う。NUL バイト自体は妥当な
// UTF-8 のため utf8.Valid だけでは UTF-16 やバイナリを見抜けず、NUL 検査で補う。
func buildPreviewResponse(body io.Reader, contentType string) (*PreviewResponse, error) {
	data, err := readPreviewBody(body)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errPreviewNotText
	}
	return &PreviewResponse{
		Content:     string(data),
		ContentType: contentType,
		Size:        int64(len(data)),
	}, nil
}

// errPreviewNotText は読み込んだ body がテキストとして扱えない (UTF-8 として不正、または
// NUL バイトを含む) ことを示す。
var errPreviewNotText = fmt.Errorf("object content appears to be binary")

// writePreviewError は buildPreviewResponse が返したエラーを適切な HTTP レスポンスへ変換する。
func writePreviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPreviewTooLarge):
		writePreviewTooLarge(w)
	case errors.Is(err, errPreviewNotText):
		writePreviewNotText(w)
	default:
		writeInternalError(w, err.Error())
	}
}
