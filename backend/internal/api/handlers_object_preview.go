package api

import (
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

// previewAllowedExtensions はプレビュー対象拡張子 (大文字小文字を区別しない)。
var previewAllowedExtensions = map[string]bool{
	".csv":  true,
	".txt":  true,
	".json": true,
}

// PreviewResponse は S3 / GCS オブジェクトプレビューの共通レスポンス。
type PreviewResponse struct {
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// previewExtensionAllowed は key の拡張子 (最終セグメントのみ、大文字小文字を区別しない) が
// プレビュー対象 (csv/txt/json) かどうかを判定する。"file.json.gz" のような多重拡張子は
// 最後の拡張子 (.gz) のみで判定するため対象外になる。
func previewExtensionAllowed(key string) bool {
	return previewAllowedExtensions[strings.ToLower(filepath.Ext(key))]
}

// previewSizeAllowed は size (メタデータ由来のオブジェクトサイズ) が maxPreviewSize 未満かを
// 判定する。TODO の要件「5 MB 以上のものはプレビュー不可」に合わせ、ちょうど 5 MB は不可とする。
func previewSizeAllowed(size int64) bool {
	return size < maxPreviewSize
}

func writePreviewUnsupportedType(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "PREVIEW_UNSUPPORTED_TYPE",
		"preview is supported only for .csv, .txt, .json files")
}

func writePreviewTooLarge(w http.ResponseWriter) {
	writeError(w, http.StatusRequestEntityTooLarge, "PREVIEW_TOO_LARGE",
		fmt.Sprintf("preview is supported only for objects under %d bytes", maxPreviewSize))
}

func writePreviewNotText(w http.ResponseWriter) {
	writeError(w, http.StatusUnprocessableEntity, "PREVIEW_NOT_TEXT",
		"object content is not valid UTF-8 text")
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

// buildPreviewResponse は拡張子ガード・サイズ上限・UTF-8 判定を経て PreviewResponse を組み立てる。
// 呼び出し側は key の拡張子ガードとサイズ事前判定 (ContentLength 等) を済ませた上で、
// body (未読み込みの Reader) と content type を渡すこと。
func buildPreviewResponse(body io.Reader, contentType string) (*PreviewResponse, error) {
	data, err := readPreviewBody(body)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errPreviewNotText
	}
	return &PreviewResponse{
		Content:     string(data),
		ContentType: contentType,
		Size:        int64(len(data)),
	}, nil
}

// errPreviewNotText は読み込んだ body が有効な UTF-8 でなかったことを示す。
var errPreviewNotText = fmt.Errorf("object content is not valid UTF-8 text")

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
