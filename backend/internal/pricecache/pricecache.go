// Package pricecache は AWS Pricing の正規化レート表を service/region 単位の
// ローカル JSON ファイルとして永続化する。TTL は設けない。ファイルが存在すれば
// 常に fresh として扱い、再取得は呼び出し側 (handler) が明示的に行う。
package pricecache

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/sync/singleflight"
)

var (
	ErrInvalidService = errors.New("invalid price cache service")
	ErrInvalidRegion  = errors.New("invalid price cache region")
)

// validServices is the fixed allowlist of thief pricing service slugs. This
// must stay in sync with the union of internal/aws's resourceServiceSpecs and
// savingsPlanServiceSpecs (issue 0055 split Savings Plans into their own
// services: compute-sp/ec2-instance-sp/database-sp), since ValidateService
// gates the on-disk cache path independently of internal/aws's own
// ValidatePricingService. Bounding it keeps the number of generated cache
// files bounded (service × region, at most a few hundred small files), so no
// cleanup job is needed.
var validServices = map[string]bool{
	"ec2":             true,
	"rds":             true,
	"elasticache":     true,
	"ecs":             true,
	"compute-sp":      true,
	"ec2-instance-sp": true,
	"database-sp":     true,
}

var validRegionRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateService returns ErrInvalidService unless service is one of the
// fixed pricing service slugs.
func ValidateService(service string) error {
	if !validServices[service] {
		return fmt.Errorf("%w: %q", ErrInvalidService, service)
	}
	return nil
}

// ValidateRegion returns ErrInvalidRegion unless region matches [a-z0-9-]+.
func ValidateRegion(region string) error {
	if !validRegionRe.MatchString(region) {
		return fmt.Errorf("%w: %q", ErrInvalidRegion, region)
	}
	return nil
}

// cacheFile is the on-disk envelope around the caller-supplied data blob.
// data の中身 (正規化レート表の JSON) は本パッケージの関知するところではない
// (internal/aws.PriceTable への依存を避けるための意図的な疎結合)。
type cacheFile struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Data      json.RawMessage `json:"data"`
}

func path(dir, service, region string) (string, error) {
	if err := ValidateService(service); err != nil {
		return "", err
	}
	if err := ValidateRegion(region); err != nil {
		return "", err
	}
	return filepath.Join(dir, service, region+".json"), nil
}

// Load reads the cached rate table bytes for service/region under dir.
// ok=false with a nil error means "no usable cache" — covering both a
// missing file (first fetch) and a corrupt/incomplete file (parse failure,
// missing fetched_at). A broken cache file must never crash the server or
// propagate as an error; it is simply treated as a miss so the caller
// re-fetches and Save overwrites it.
func Load(dir, service, region string) (data []byte, fetchedAt time.Time, ok bool, err error) {
	p, verr := path(dir, service, region)
	if verr != nil {
		return nil, time.Time{}, false, verr
	}
	raw, ferr := os.ReadFile(p)
	if os.IsNotExist(ferr) {
		return nil, time.Time{}, false, nil
	}
	if ferr != nil {
		return nil, time.Time{}, false, fmt.Errorf("read price cache %s: %w", p, ferr)
	}
	var cf cacheFile
	if jerr := json.Unmarshal(raw, &cf); jerr != nil {
		slog.Warn("discard corrupt price cache file", "path", p, "err", jerr)
		return nil, time.Time{}, false, nil
	}
	if cf.FetchedAt.IsZero() || len(cf.Data) == 0 || string(cf.Data) == "null" {
		slog.Warn("discard incomplete price cache file", "path", p)
		return nil, time.Time{}, false, nil
	}
	return []byte(cf.Data), cf.FetchedAt, true, nil
}

// Save atomically writes data (already-marshalled JSON) plus fetchedAt to
// dir/service/region.json. It writes a temp file in the same directory
// first, then renames it into place, so a concurrent Load never observes a
// partially written file.
func Save(dir, service, region string, data []byte, fetchedAt time.Time) error {
	p, err := path(dir, service, region)
	if err != nil {
		return err
	}
	destDir := filepath.Dir(p)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return fmt.Errorf("create price cache dir %s: %w", destDir, err)
	}
	payload, err := json.Marshal(cacheFile{FetchedAt: fetchedAt, Data: json.RawMessage(data)})
	if err != nil {
		return fmt.Errorf("marshal price cache: %w", err)
	}
	tmp, err := os.CreateTemp(destDir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp price cache file in %s: %w", destDir, err)
	}
	defer os.Remove(tmp.Name()) // rename 成功後は ENOENT になるだけなので常に呼んでよい
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return fmt.Errorf("write price cache %s: %w", p, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close price cache temp file: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("chmod price cache temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return fmt.Errorf("rename price cache %s: %w", p, err)
	}
	return nil
}

// fetchGroup dedupes concurrent misses/refreshes of the same dir/service/
// region. It is package-level (not per-call) because Load/Save are free
// functions with no long-lived instance to hold it; a single server process
// has exactly one PriceCacheDir, so a package-level group is equivalent to
// one instance per process.
var fetchGroup singleflight.Group

// Fetch runs loader under singleflight keyed by dir/service/region, so N
// concurrent requests for an uncached (or explicitly refreshed) table
// trigger exactly one loader call. It does not itself read or write the
// cache; callers combine it with Load/Save.
func Fetch(dir, service, region string, loader func() ([]byte, error)) ([]byte, error) {
	key := dir + "|" + service + "|" + region
	v, err, _ := fetchGroup.Do(key, func() (any, error) {
		return loader()
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}
