package aws

import "strings"

// Resource is the common interface for all AWS resource types.
type Resource interface {
	ResourceID() string
	ResourceName() string
	// ResourceState returns a normalized state:
	// running | stopped | available | active | error | stale | updating | unknown
	ResourceState() string
	ServiceName() string
}

// NormalizeState maps service-specific state strings to normalized values.
func NormalizeState(raw string) string {
	switch raw {
	case "running", "RUNNING", "active", "ACTIVE", "available", "AVAILABLE",
		"enabled", "ENABLED", "deployed", "DEPLOYED":
		return "running"
	case "stopped", "STOPPED", "inactive", "INACTIVE", "disabled", "DISABLED":
		return "stopped"
	case "pending", "PENDING", "provisioning", "PROVISIONING",
		"creating", "CREATING", "starting", "STARTING":
		return "updating"
	case "error", "ERROR", "failed", "FAILED", "impaired", "IMPAIRED":
		return "error"
	case "updating", "UPDATING", "modifying", "MODIFYING", "rebooting", "REBOOTING":
		return "updating"
	default:
		return raw
	}
}

// tagsToMapFunc は SDK ごとに型が異なる Tag スライスを map[string]string へ変換する。
// kv は各 Tag からキーと値のポインタを取り出すアクセサ。nil は空文字として扱う。
func tagsToMapFunc[T any](tags []T, kv func(T) (key, value *string)) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		k, v := kv(t)
		m[ptrStr(k)] = ptrStr(v)
	}
	return m
}

// DisplayState は SDK 由来の state 文字列を JSON/UI 表示用に小文字・ハイフン正規化する。
// 意味は丸めない (available と running は別値のまま保持する)。
func DisplayState(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "inprogress": // CloudFront "InProgress" は区切り無しパスカルのため明示変換する
		return "in-progress"
	}
	return s
}
