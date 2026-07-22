package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// errValueUpdateNameRequired は value-update リクエストの name が空のときに返す。
var errValueUpdateNameRequired = errors.New("name is required")

// parseValueUpdate は Secrets Manager / SSM の値更新リクエストボディをデコードして検証する。
// name は必須、value は空文字列も許容する (空値への更新を許すため)。
func parseValueUpdate(body io.Reader) (ValueUpdateRequest, error) {
	var req ValueUpdateRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return req, fmt.Errorf("invalid request body: %w", err)
	}
	if req.Name == "" {
		return req, errValueUpdateNameRequired
	}
	return req, nil
}

// handleSecretsPut は Secrets Manager のシークレット値を更新する。
// 更新後は一覧キャッシュを無効化して次回取得で新しい値を反映させる。
func (s *Server) handleSecretsPut(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	req, err := parseValueUpdate(r.Body)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if err := awsinternal.PutSecretValue(r.Context(), profile, region, req.Name, req.Value); err != nil {
		writeAWSError(w, err)
		return
	}
	s.resourceCache.Invalidate(cacheKey("secretsmanager-list", profile, region))
	w.WriteHeader(http.StatusNoContent)
}

// handleSSMPut は SSM Parameter Store のパラメータ値を更新する。
// 更新後は一覧キャッシュを無効化して次回取得で新しい値を反映させる。
func (s *Server) handleSSMPut(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	req, err := parseValueUpdate(r.Body)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if err := awsinternal.PutSSMParameter(r.Context(), profile, region, req.Name, req.Value); err != nil {
		writeAWSError(w, err)
		return
	}
	s.resourceCache.Invalidate(cacheKey("ssm-list", profile, region))
	w.WriteHeader(http.StatusNoContent)
}
