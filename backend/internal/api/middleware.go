package api

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/config"
)

// maxErrorBodyLogBytes は 4xx / 5xx 応答のログに載せるボディの最大バイト数。
// writeError の JSON はエラーコードとメッセージのみで通常は数百バイトに収まるため、
// 2048 バイトあれば切り詰めなしで写せる。上限はリクエストごとのバッファの上限でも
// あり、エラー応答が並行して多数発生してもメモリ使用量を上限 × 同時応答数に抑える。
const maxErrorBodyLogBytes = 2048

// loggingMiddleware logs each request's method, path, status, and duration.
// 4xx / 5xx の応答では Info のアクセスログの代わりに、捕捉した応答ボディを
// 添えて 4xx は Warn、5xx は Error で 1 行出す。
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		switch {
		case rw.status >= http.StatusInternalServerError:
			slog.Error("http", append(attrs, "body", string(rw.body))...)
		case rw.status >= http.StatusBadRequest:
			slog.Warn("http", append(attrs, "body", string(rw.body))...)
		default:
			slog.Info("http", attrs...)
		}
	})
}

// corsAllowMethods は許可した Origin に返す Access-Control-Allow-Methods の値。
// DELETE ルート (Athena クエリ停止、BigQuery ジョブキャンセル、スニペット削除) の
// プリフライトを通過させるため含める。
const corsAllowMethods = "GET, POST, DELETE, OPTIONS"

// corsAllowHeaders は許可した Origin に返す Access-Control-Allow-Headers の値。
const corsAllowHeaders = "Content-Type"

// defaultHTTPPort は http の URI の既定ポート (RFC 9110 4.2.1)。
const defaultHTTPPort = "80"

// corsMiddleware adds CORS headers only for web clients whose Origin is allowed.
//
// 許可の判定は websocket.Accept (github.com/coder/websocket の authenticateOrigin) と
// 同じ規則で行い、HTTP の API と WebSocket で許可する Origin の範囲を揃える。
// 許可しない Origin には CORS のヘッダを返さず、ブラウザに応答を読ませない。さらに、
// 状態を変えるメソッド (GET / HEAD / OPTIONS 以外) はハンドラに渡さず 403 で拒否する。
// CORS はプリフライトの無い単純リクエスト (multipart/form-data の POST など) の送信自体を
// 止めないためである。Origin の無いリクエスト (curl などのブラウザ以外のクライアント、
// トップレベルナビゲーションの GET) は従来どおり通す。
func corsMiddleware(next http.Handler, originPatterns []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := origin != "" && originAllowed(r, originPatterns)
		if origin != "" {
			// 応答が Origin によって変わることを共有キャッシュに伝える。許可の有無を問わず付ける。
			w.Header().Set("Vary", "Origin")
		}
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		}
		if r.Method == http.MethodOptions {
			// プリフライト。許可しない Origin には CORS のヘッダが無いため、ブラウザは本リクエストを送らない。
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if origin != "" && !allowed && r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusForbidden, "FORBIDDEN_ORIGIN", "request Origin is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed はリクエストの Origin が許可されるかを、websocket.Accept の
// authenticateOrigin と同じ規則で判定する。Origin を URL として解析し、ホスト部
// (host:port) が r.Host と大文字小文字を無視して一致するか、originPatterns のいずれかの
// パターンに path.Match (パターンと対象を小文字にして照合) で一致すれば許可する。
// パターンが "://" を含む場合は scheme://host と照合する。
// パターンは先頭から順に照合し、不正なパターン (path.ErrBadPattern) に当たった時点で
// 許可しない。websocket.Accept も同じ順で照合し、不正なパターンに当たるとログを出して
// 403 で拒否するため、ログの出力も含めて揃える。不正なパターンより前に一致したパターンが
// あれば許可する。
func originAllowed(r *http.Request, originPatterns []string) bool {
	u, err := url.Parse(r.Header.Get("Origin"))
	if err != nil {
		return false
	}
	if strings.EqualFold(r.Host, u.Host) {
		return true
	}
	for _, pattern := range originPatterns {
		target := u.Host
		if strings.Contains(pattern, "://") {
			target = u.Scheme + "://" + u.Host
		}
		matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(target))
		if err != nil {
			slog.Warn("invalid web origin pattern", "pattern", pattern, "err", err)
			return false
		}
		if matched {
			return true
		}
	}
	return false
}

// hostCheckMiddleware rejects requests whose Host is not in allowedHosts with
// 421 Misdirected Request.
//
// DNS リバインディングでは攻撃者のドメインが Host になる。その場合 Origin のホストと
// Host が一致して同一オリジンとして通るため、Origin の検査だけでは防げない。Host の検査を
// corsMiddleware より外側に置くのはこのためである。RFC 9110 15.5.20 は、対象 URI に
// 対して権威のある応答を返せないサーバが 421 Misdirected Request を返すと定める。
func hostCheckMiddleware(next http.Handler, allowedHosts []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, allowedHosts) {
			writeError(w, http.StatusMisdirectedRequest, "MISDIRECTED_REQUEST", "request Host is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostAllowed は host が許可するホストの一覧に含まれるか (大文字小文字を無視) を返す。
func hostAllowed(host string, allowedHosts []string) bool {
	for _, allowed := range allowedHosts {
		if strings.EqualFold(host, allowed) {
			return true
		}
	}
	return false
}

// allowedHostsFromConfig は Host ヘッダの検査で許可するホストの一覧を cfg から組み立てる。
// 待ち受けポートの loopback (localhost / 127.0.0.1 / [::1])、待ち受けアドレスそのもの
// (未指定アドレスを除く)、Datadog OAuth のコールバックへブラウザが到達する URL のホスト部
// である (ADR 0032)。比較は hostAllowed が大文字小文字を無視して行う。
//
// 待ち受けポートが http の既定ポート 80 のときは、ポートを省いた形も加える。RFC 9110 の
// 4.2.3 は既定ポートを省いた形を http の URI の正規形とし、ブラウザはその URI のホスト部を
// Host (7.2) に送るためである。
func allowedHostsFromConfig(cfg *config.Config) []string {
	host, port, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		// 空の ListenAddr では net/http が ":http" (ポート 80) で待ち受ける。ポートの無い空でない
		// 値は net.Listen が "missing port in address" で拒否し、サーバが起動しないため、
		// 許可するホストの組み立てでは空と同じく扱う。
		host, port = "", defaultHTTPPort
	}
	var allowed []string
	add := func(h string) {
		if h == "" || hostAllowed(h, allowed) {
			return
		}
		allowed = append(allowed, h)
	}
	addHost := func(h string) {
		hostPort := net.JoinHostPort(h, port)
		add(hostPort)
		if port == defaultHTTPPort {
			// JoinHostPort が IPv6 のリテラルを角括弧で囲むため、ポートの前までを使う。
			add(strings.TrimSuffix(hostPort, ":"+port))
		}
	}
	for _, loopback := range []string{"localhost", "127.0.0.1", "::1"} {
		addHost(loopback)
	}
	// 未指定アドレス (0.0.0.0 / ::) は特定のホストを表さないため、待ち受けアドレス自体は
	// 加えない。LAN の他の端末から IP アドレスで開く使い方は、OAuthRedirectBase にその URL を
	// 設定しない限り 421 になる (ADR 0032 の結果に記録済み)。
	if host != "" && host != "0.0.0.0" && host != "::" {
		addHost(host)
	}
	if u, err := url.Parse(cfg.Datadog.OAuthRedirectBase); err == nil {
		add(u.Host)
	}
	return allowed
}

type responseWriter struct {
	http.ResponseWriter
	status int
	// body は 4xx / 5xx 応答のボディの写し (先頭 maxErrorBodyLogBytes バイトまで)。
	// WriteHeader を経ずにボディが書かれた応答は status が既定の 200 のままなので
	// 捕捉対象にならない。
	body []byte
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// Write はボディをそのまま透過しつつ、4xx / 5xx の場合だけ上限まで写しを蓄積する。
// 複数回の呼び出しをまたいで累積し、上限に達した以降は捕捉だけを打ち切る。
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status >= http.StatusBadRequest && len(rw.body) < maxErrorBodyLogBytes {
		n := maxErrorBodyLogBytes - len(rw.body)
		if n > len(b) {
			n = len(b)
		}
		rw.body = append(rw.body, b[:n]...)
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying http.ResponseWriter so that http.ResponseController
// (and libraries following the same convention, e.g. websocket.Accept's hijack lookup)
// can reach interfaces like http.Hijacker through this wrapper.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
