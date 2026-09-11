package api

import (
	"sync"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// datadogLoginSessionTTL は login/start が作ったセッションの保持期間。ブラウザでの
// 承認を待つ時間に加え、callback の結果を login/status がポーリングで取りに来る猶予を
// 含む。CLI のログイン上限 (datadogAuthLoginTimeout、5 分) より長くする。
const datadogLoginSessionTTL = 10 * time.Minute

// datadogLoginStatus は login/status が返す進行状態。
type datadogLoginStatus string

const (
	datadogLoginPending   datadogLoginStatus = "pending"
	datadogLoginSucceeded datadogLoginStatus = "succeeded"
	datadogLoginFailed    datadogLoginStatus = "failed"
)

// datadogLoginSession は login/start が作成し、callback が消費して結果を書き込み、
// login/status が読み取る進行中のログイン。
type datadogLoginSession struct {
	// login は認可の中間状態 (PKCE の code_verifier と state を含む)。callback が
	// take で取り出した時点で nil になり、同じ state の 2 回目の callback は
	// 未知のセッションとして弾かれる (認可コードの再送を 1 回だけ有効にする)。
	login *datadogauth.Login

	status datadogLoginStatus
	// errMsg は status が failed のときの理由。
	errMsg    string
	expiresAt time.Time
}

// datadogLoginSessionStore は進行中の Datadog ログインをメモリに保持する。
// 永続化はしない (ssoLoginSessionStore と同じ判断: backend はローカル常駐の単一
// プロセスであり、再起動で消えても frontend がログインをやり直せば足りる)。
type datadogLoginSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*datadogLoginSession

	// now は失効判定に使う現在時刻を返す。テストから固定時刻を注入できるようにする。
	now func() time.Time
}

func newDatadogLoginSessionStore() *datadogLoginSessionStore {
	return &datadogLoginSessionStore{
		sessions: map[string]*datadogLoginSession{},
		now:      time.Now,
	}
}

// put は login をセッションとして保存する。セッション ID には認可要求の state
// (datadogauth が crypto/rand で生成した推測不能な値) をそのまま使う。callback は
// state しか持ち帰らないため、別の ID を振ると突き合わせる手段が無い。
// あわせて失効済みのセッションを掃除する (定期タスクを持たず、アクセス時に破棄する)。
func (st *datadogLoginSessionStore) put(login *datadogauth.Login) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sweepLocked()
	st.sessions[login.State] = &datadogLoginSession{
		login:     login,
		status:    datadogLoginPending,
		expiresAt: st.now().Add(datadogLoginSessionTTL),
	}
}

// take は state に対応する認可の中間状態を取り出し、セッションからは取り除く。
// 取り出しと取り除きを不可分に行い、同じ state の callback を 1 回だけ有効にする。
// セッション自体は結果の受け皿として残す (login/status が読む)。
func (st *datadogLoginSessionStore) take(state string) (*datadogauth.Login, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sweepLocked()
	sess, ok := st.sessions[state]
	if !ok || sess.login == nil {
		return nil, false
	}
	login := sess.login
	sess.login = nil
	return login, true
}

// finish は callback の結果をセッションへ書き込む。err が nil なら成功とする。
func (st *datadogLoginSessionStore) finish(state string, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	sess, ok := st.sessions[state]
	if !ok {
		return
	}
	if err != nil {
		sess.status = datadogLoginFailed
		sess.errMsg = err.Error()
		return
	}
	sess.status = datadogLoginSucceeded
	sess.errMsg = ""
}

// lookup は state の進行状態を返す。未知または失効したセッションは ok=false。
func (st *datadogLoginSessionStore) lookup(state string) (datadogLoginStatus, string, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sweepLocked()
	sess, ok := st.sessions[state]
	if !ok {
		return "", "", false
	}
	return sess.status, sess.errMsg, true
}

// sweepLocked は失効したセッションを破棄する。st.mu を保持した状態で呼ぶこと。
func (st *datadogLoginSessionStore) sweepLocked() {
	now := st.now()
	for state, sess := range st.sessions {
		if !sess.expiresAt.After(now) {
			delete(st.sessions, state)
		}
	}
}
