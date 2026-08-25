package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/ssoauth"
)

// ssoLoginSessionDefaultTTL は StartDeviceAuthorization が expiresIn を指示しなかった
// (0 以下の) 場合のセッション保持期間。RFC 8628 §3.2 で expires_in は REQUIRED であり
// 指示が無いのは仕様に従っていない応答だが、その場合でも失効しないセッションを
// 溜め込まないよう上限を置く。値は internal/aws のポーリング打ち切り既定
// (ssoTokenPollDefaultTimeout) と同じ 600 秒に合わせる。
const ssoLoginSessionDefaultTTL = 600 * time.Second

// ssoLoginSessionIDBytes はセッション ID に使う乱数のバイト長。ID は complete を呼ぶ
// 権利そのものなので推測不能でなければならない。256 bit あれば総当たりは非現実的。
const ssoLoginSessionIDBytes = 32

// ssoLoginSession は start が作成し complete が消費する、進行中のデバイス認可の
// 中間状態。profile は complete のパスの profile と突き合わせるために持つ。
type ssoLoginSession struct {
	profile   string
	sess      *ssoauth.Session
	expiresAt time.Time
}

// ssoLoginSessionStore は進行中のログインセッションをメモリに保持する。
// 永続化はしない (issue 0148 の設計判断: backend はローカル常駐の単一プロセスであり、
// 再起動で消えても frontend がログインをやり直せば足りる)。
type ssoLoginSessionStore struct {
	mu       sync.Mutex
	sessions map[string]ssoLoginSession

	// now は失効判定に使う現在時刻を返す。テストから固定時刻を注入できるようにする。
	now func() time.Time

	// randRead はセッション ID の乱数生成に使う。crypto/rand の失敗パスを
	// テストから再現できるようにするために注入可能にする。
	randRead func(b []byte) (int, error)
}

func newSSOLoginSessionStore() *ssoLoginSessionStore {
	return &ssoLoginSessionStore{
		sessions: map[string]ssoLoginSession{},
		now:      time.Now,
		randRead: rand.Read,
	}
}

// put は新しいセッションを保存し、crypto/rand 由来のセッション ID を返す。
// 有効期限はデバイス認可の expiresIn に合わせる (device code が失効した後の complete は
// どのみち成功しないため、セッションを残しても意味が無い)。あわせて失効済みの
// セッションを掃除する (バックグラウンドの定期タスクを持たず、アクセス時に破棄する)。
func (st *ssoLoginSessionStore) put(profile string, sess *ssoauth.Session) (string, error) {
	buf := make([]byte, ssoLoginSessionIDBytes)
	if _, err := st.randRead(buf); err != nil {
		return "", fmt.Errorf("generate sso login session id: %w", err)
	}
	id := hex.EncodeToString(buf)

	ttl := ssoLoginSessionDefaultTTL
	if sess != nil && sess.DeviceAuth != nil && sess.DeviceAuth.ExpiresIn > 0 {
		ttl = time.Duration(sess.DeviceAuth.ExpiresIn) * time.Second
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	st.sweepLocked()
	st.sessions[id] = ssoLoginSession{
		profile:   profile,
		sess:      sess,
		expiresAt: st.now().Add(ttl),
	}
	return id, nil
}

// take は profile に紐づくセッションを取り出し、マップから削除する。同一セッション ID
// の complete を 1 回だけ有効にするため、取り出しと削除は不可分に行う。失効済みの
// セッションは存在しないものとして扱う (sweepLocked が先に破棄する)。
// profile が一致しないセッションは削除せずに false を返す。誤った profile への
// complete (frontend の不整合など) が正規の complete の機会を潰さないようにするため。
func (st *ssoLoginSessionStore) take(profile, id string) (ssoLoginSession, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sweepLocked()
	entry, ok := st.sessions[id]
	if !ok || entry.profile != profile {
		return ssoLoginSession{}, false
	}
	delete(st.sessions, id)
	return entry, true
}

// sweepLocked は失効したセッションを破棄する。st.mu を保持した状態で呼ぶこと。
func (st *ssoLoginSessionStore) sweepLocked() {
	now := st.now()
	for id, entry := range st.sessions {
		if !entry.expiresAt.After(now) {
			delete(st.sessions, id)
		}
	}
}
