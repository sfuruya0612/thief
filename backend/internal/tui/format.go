package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// formatCount は件数を Web UI の toLocaleString() と同じ 3 桁ごとの "," 区切りにする (1234567 は "1,234,567")。
// Web UI は en と ja のどちらのロケールでも "," 区切りになるため、"," に固定する。
func formatCount(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// formatBytesUnits は formatBytes の単位。Web UI の formatBytes (frontend/src/components/tables/columns.tsx) と同じ。
var formatBytesUnits = []string{"B", "KB", "MB", "GB", "TB"}

// formatBytes は容量を Web UI の formatBytes と同じ規則で返す。0 以下は "0 B"、1024 未満は "<n> B"、
// それ以上は 1024 で割って小数 1 桁と単位 (KB、MB、GB、TB) にする。
// 小数 1 桁への丸めは JavaScript の toFixed(1) と同じく 0.5 を切り上げる (1280 は "1.3 KB")。
// Go の %.1f は偶数へ丸めるため、そのままでは "1.2 KB" になる。
func formatBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(formatBytesUnits)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	rounded := math.Floor(v*10+0.5) / 10
	return fmt.Sprintf("%.1f %s", rounded, formatBytesUnits[i])
}

// formatInt32 は件数やポート番号を Web UI と同じ区切り無しの 10 進表記にする。
func formatInt32(n int32) string {
	return strconv.FormatInt(int64(n), 10)
}

// fetchFailed は項目ごとの取得失敗 (IAM の MFA、Policies、Groups と WAF の Associated) を値の文字列で示す。
// Web UI は値の横に警告アイコン (frontend/src/components/primitives/FetchFailedWarning.tsx) を出すが、
// TUI は value が空なら "fetch failed"、空でなければ "<value> (fetch failed)" を返す。
func fetchFailed(value string) string {
	if value == "" {
		return "fetch failed"
	}
	return value + " (fetch failed)"
}
