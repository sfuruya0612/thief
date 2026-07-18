package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/pricecache"
)

// handlePricing serves the normalized price table for one profile/service/
// region. Unlike other AWS resource handlers, pricing is cached as local
// files (internal/pricecache), not in s.resourceCache: rates are
// account-independent and don't expire, so a filesystem cache with no TTL
// is a better fit than the in-memory TTL cache the other handlers share.
func (s *Server) handlePricing(w http.ResponseWriter, r *http.Request) {
	// profile はキャッシュファイルパスの構築に使わず (キャッシュキーは service/region の
	// み)、AWS SDK の認証にのみ使う。他のリソースハンドラ (handleEC2 等) と同じく
	// profileAndRegion 経由の素通しとし、ValidateProfileName は適用しない。
	// ValidateProfileName の許可文字集合 [A-Za-z0-9_-] は "CT Audit" のようなスペースを
	// 含む実在のプロファイル名を拒否してしまい、他のハンドラでは発生しない 400 エラーに
	// なることが実ブラウザ確認で判明した。
	profile := r.PathValue("profile")
	service := r.URL.Query().Get("service")
	if err := awsinternal.ValidatePricingService(service); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	region := r.URL.Query().Get("region")
	if err := pricecache.ValidateRegion(region); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	dir := s.cfg.PriceCacheDir

	if !s.refresh(r) {
		data, _, ok, err := pricecache.Load(dir, service, region)
		if err != nil {
			// キャッシュ I/O エラーは絶対パス等の詳細をクライアントへ返さず、
			// サーバ側にのみ記録する。
			slog.Error("load price cache failed", "service", service, "region", region, "err", err)
			writeInternalError(w, "failed to read price cache")
			return
		}
		if ok {
			writeJSONBytes(w, data)
			return
		}
	}

	var fetchErr error // GetPricing 由来のエラーかどうかをキャッシュ I/O エラーと区別する
	data, err := pricecache.Fetch(dir, service, region, func() ([]byte, error) {
		table, gerr := awsinternal.GetPricing(r.Context(), profile, region, service)
		if gerr != nil {
			fetchErr = gerr
			return nil, gerr
		}
		table.FetchedAt = time.Now().UTC()
		payload, merr := json.Marshal(table)
		if merr != nil {
			return nil, merr
		}
		if serr := pricecache.Save(dir, service, region, payload, table.FetchedAt); serr != nil {
			return nil, serr
		}
		return payload, nil
	})
	if err != nil {
		if fetchErr != nil {
			writePricingError(w, fetchErr)
			return
		}
		slog.Error("persist price cache failed", "service", service, "region", region, "err", err)
		writeInternalError(w, "failed to persist price cache")
		return
	}
	writeJSONBytes(w, data)
}

func writeJSONBytes(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
