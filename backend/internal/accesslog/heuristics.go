package accesslog

import "strings"

// sensitivePaths は攻撃の初期探索でよく要求されるパスの一覧である。
// この一覧は悪意の判定ではなく、集計結果を絞り込むための材料として使う。
var sensitivePaths = []string{
	"/.env",
	"/.git",
	"/.aws",
	"/.ssh",
	"/.ds_store",
	"/wp-login.php",
	"/wp-admin",
	"/xmlrpc.php",
	"/phpmyadmin",
	"/pma",
	"/adminer",
	"/actuator",
	"/server-status",
	"/cgi-bin",
	"/vendor/phpunit",
	"/console",
	"/manager/html",
	"/solr",
	"/jenkins",
	"/config.json",
	"/backup",
}

// IsSensitivePath は URI のパスが、攻撃の初期探索でよく要求されるパスに一致するかを返す。
func IsSensitivePath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	for _, candidate := range sensitivePaths {
		if path == candidate || strings.HasPrefix(path, candidate+"/") {
			return true
		}
	}
	return false
}

// SensitivePaths は判定に使うパスの一覧を返す。
func SensitivePaths() []string {
	out := make([]string, len(sensitivePaths))
	copy(out, sensitivePaths)
	return out
}
