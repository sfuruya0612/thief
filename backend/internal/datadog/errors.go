package datadog

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

// IsForbidden は err が HTTP 403 を伴う Datadog API のエラーかどうかを返す。
//
// datadog.GenericOpenAPIError はステータスコード自体を保持しない。非 2xx の
// レスポンスでは SDK が http.Response.Status ("403 Forbidden") を ErrorMessage に、
// 生のボディを ErrorBody に入れる。そのため ErrorMessage の先頭のトークンだけが
// ステータスコードの残る場所になる。
func IsForbidden(err error) bool {
	var apiErr datadog.GenericOpenAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code, _, _ := strings.Cut(apiErr.ErrorMessage, " ")
	return code == strconv.Itoa(http.StatusForbidden)
}

// ErrorBody は Datadog API のエラーの生のレスポンスボディを maxLen バイトまで
// 切り詰めて返す。err が Datadog API のエラーでない場合やボディを持たない場合は
// 空文字列を返す。GenericOpenAPIError.Error() はステータス行しか返さないため、
// 呼び出し側は呼び出しが拒否された理由をログに残すためにこれを使う。
func ErrorBody(err error, maxLen int) string {
	var apiErr datadog.GenericOpenAPIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	body := strings.TrimSpace(string(apiErr.ErrorBody))
	if len(body) > maxLen {
		return body[:maxLen]
	}
	return body
}
