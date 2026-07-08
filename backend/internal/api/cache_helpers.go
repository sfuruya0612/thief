package api

import (
	"time"

	"github.com/sfuruya0612/thief/backend/internal/cache"
)

func cacheHeadersFrom(hit bool, entry cache.Entry[any]) CacheHeaders {
	status := "MISS"
	if hit {
		status = "HIT"
	}
	return CacheHeaders{
		Status:    status,
		CachedAt:  entry.CachedAt,
		ExpiresAt: entry.Expiry,
		TTL:       int(time.Until(entry.Expiry).Seconds()),
	}
}
