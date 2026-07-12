package api

import (
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func (s *Server) handleDynamoSchema(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	table := r.PathValue("table")
	key := cacheKey("dynamo-schema", profile, region, table)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.DescribeDynamoTable(r.Context(), profile, region, table)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleDynamoItems(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	table := r.PathValue("table")
	req := awsinternal.DynamoItemQuery{
		PKValue:   r.URL.Query().Get("pk_val"),
		SKValue:   r.URL.Query().Get("sk_val"),
		AttrName:  r.URL.Query().Get("attr_name"),
		AttrValue: r.URL.Query().Get("attr_val"),
	}
	// PK/SK/属性フィルタの値そのものをキャッシュキーに含める (Query/Scan 結果は入力ごとに変わる)。
	key := cacheKey("dynamo-items", profile, region, table, req.PKValue, req.SKValue, req.AttrName, req.AttrValue)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.QueryDynamoItems(r.Context(), profile, region, table, req)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}
