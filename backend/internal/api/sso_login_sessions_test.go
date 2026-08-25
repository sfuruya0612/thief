package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/ssoauth"
)

// testSSOAuthSession は ExpiresIn を指定してテスト用の ssoauth.Session を組む。
func testSSOAuthSession(expiresIn int32) *ssoauth.Session {
	return &ssoauth.Session{
		Region:       "ap-northeast-1",
		StartURL:     "https://example.awsapps.com/start",
		Registration: &awsinternal.SSOClientRegistration{ClientID: "cid", ClientSecret: "secret"},
		DeviceAuth: &awsinternal.SSODeviceAuthorization{
			DeviceCode: "device-code",
			UserCode:   "USER-CODE",
			ExpiresIn:  expiresIn,
		},
	}
}

// TestSSOLoginSessionStorePutAndTake は保存したセッションが同じ ID で 1 回だけ
// 取り出せること (one-shot) を検証する。
func TestSSOLoginSessionStorePutAndTake(t *testing.T) {
	st := newSSOLoginSessionStore()
	sess := testSSOAuthSession(600)

	id, err := st.put("dev", sess)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	// 32 バイトの乱数の hex 表現なので 64 文字になる。
	if len(id) != ssoLoginSessionIDBytes*2 {
		t.Fatalf("id length = %d, want %d", len(id), ssoLoginSessionIDBytes*2)
	}

	entry, ok := st.take("dev", id)
	if !ok {
		t.Fatal("take = false, want true")
	}
	if entry.profile != "dev" {
		t.Errorf("profile = %q, want %q", entry.profile, "dev")
	}
	if entry.sess != sess {
		t.Errorf("sess = %p, want the same session %p", entry.sess, sess)
	}

	// 2 回目の取り出しは失敗する (complete は同一 ID に対して 1 回だけ有効)。
	if _, ok := st.take("dev", id); ok {
		t.Error("second take = true, want false")
	}
}

// TestSSOLoginSessionStoreTakeUnknownID は保存していない ID の取り出しが失敗する
// ことを検証する。
func TestSSOLoginSessionStoreTakeUnknownID(t *testing.T) {
	st := newSSOLoginSessionStore()
	if _, ok := st.take("dev", "unknown"); ok {
		t.Error("take = true, want false")
	}
}

// TestSSOLoginSessionStoreTakeProfileMismatch は profile が一致しない取り出しが失敗し、
// かつセッションを消費しないことを検証する。誤った profile への complete が正規の
// complete の機会を潰さないための挙動 (issue 0148 のレビュー指摘)。
func TestSSOLoginSessionStoreTakeProfileMismatch(t *testing.T) {
	st := newSSOLoginSessionStore()
	sess := testSSOAuthSession(600)

	id, err := st.put("dev", sess)
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, ok := st.take("prod", id); ok {
		t.Error("take with mismatched profile = true, want false")
	}

	// 不一致の取り出しではセッションが消費されず、正しい profile で取り出せる。
	entry, ok := st.take("dev", id)
	if !ok {
		t.Fatal("take with correct profile after mismatch = false, want true")
	}
	if entry.sess != sess {
		t.Errorf("sess = %p, want the same session %p", entry.sess, sess)
	}
}

// TestSSOLoginSessionStorePutRandError は乱数生成の失敗が呼び出し元へエラーとして
// 伝播することを検証する。
func TestSSOLoginSessionStorePutRandError(t *testing.T) {
	st := newSSOLoginSessionStore()
	st.randRead = func(_ []byte) (int, error) {
		return 0, errors.New("rand broken")
	}

	if _, err := st.put("dev", testSSOAuthSession(600)); err == nil {
		t.Fatal("put = nil error, want error")
	}
	if len(st.sessions) != 0 {
		t.Errorf("sessions size = %d, want 0 (failed put must not store anything)", len(st.sessions))
	}
}

// TestSSOLoginSessionStoreExpiry はデバイス認可の expiresIn を過ぎたセッションが
// 取り出せず、失効時に破棄されることを検証する。
func TestSSOLoginSessionStoreExpiry(t *testing.T) {
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	now := base
	st := newSSOLoginSessionStore()
	st.now = func() time.Time { return now }

	id, err := st.put("dev", testSSOAuthSession(900))
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	// 期限ちょうどは失効扱い (expiresAt.After(now) が false になる境界)。
	now = base.Add(900 * time.Second)
	if _, ok := st.take("dev", id); ok {
		t.Error("take after expiry = true, want false")
	}
	if len(st.sessions) != 0 {
		t.Errorf("sessions size = %d, want 0 (expired entries are swept)", len(st.sessions))
	}
}

// TestSSOLoginSessionStoreDefaultTTL は expiresIn の指示が無い (0 以下の) セッションが
// 既定の保持期間 (ssoLoginSessionDefaultTTL) で失効することを検証する。
func TestSSOLoginSessionStoreDefaultTTL(t *testing.T) {
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	now := base
	st := newSSOLoginSessionStore()
	st.now = func() time.Time { return now }

	id, err := st.put("dev", testSSOAuthSession(0))
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	// 既定 TTL の直前ではまだ有効。
	now = base.Add(ssoLoginSessionDefaultTTL - time.Second)
	if _, ok := st.take("dev", id); !ok {
		t.Fatal("take before default TTL = false, want true")
	}

	// 取り出し直して既定 TTL 経過後は失効する。
	now = base
	id2, err := st.put("dev", testSSOAuthSession(-1))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	now = base.Add(ssoLoginSessionDefaultTTL)
	if _, ok := st.take("dev", id2); ok {
		t.Error("take after default TTL = true, want false")
	}
}

// TestSSOLoginSessionStoreConcurrentSessions は同一 profile の複数セッションが独立に
// 保持され、互いを上書きしないことを検証する (issue 0148 の設計判断)。
func TestSSOLoginSessionStoreConcurrentSessions(t *testing.T) {
	st := newSSOLoginSessionStore()
	sess1 := testSSOAuthSession(600)
	sess2 := testSSOAuthSession(600)

	id1, err := st.put("dev", sess1)
	if err != nil {
		t.Fatalf("put sess1: %v", err)
	}
	id2, err := st.put("dev", sess2)
	if err != nil {
		t.Fatalf("put sess2: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("session ids collide: %q", id1)
	}

	e1, ok := st.take("dev", id1)
	if !ok || e1.sess != sess1 {
		t.Errorf("take(id1) = (%p, %t), want (%p, true)", e1.sess, ok, sess1)
	}
	e2, ok := st.take("dev", id2)
	if !ok || e2.sess != sess2 {
		t.Errorf("take(id2) = (%p, %t), want (%p, true)", e2.sess, ok, sess2)
	}
}

// TestSSOLoginSessionStoreParallelAccess は複数 goroutine からの put / take を実際に
// 並行実行し、-race によるデータ競合検出とロック漏れの回帰検出に掛ける。
// 同一 profile への同時 put と、同一セッション ID への同時 take (one-shot の競合)
// という衝突するパターンを含める。
func TestSSOLoginSessionStoreParallelAccess(t *testing.T) {
	const workers = 16
	st := newSSOLoginSessionStore()

	// 同一 profile への同時 put と、各自が put した ID の take を並行実行する。
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := st.put("dev", testSSOAuthSession(600))
			if err != nil {
				t.Errorf("put: %v", err)
				return
			}
			if _, ok := st.take("dev", id); !ok {
				t.Errorf("take(dev, %s) = false, want true", id)
			}
		}()
	}
	wg.Wait()

	// 同一セッション ID への同時 take は 1 goroutine だけが成功する (one-shot)。
	id, err := st.put("dev", testSSOAuthSession(600))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	var succeeded atomic.Int32
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := st.take("dev", id); ok {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := succeeded.Load(); got != 1 {
		t.Errorf("concurrent take succeeded %d times, want exactly 1", got)
	}

	if len(st.sessions) != 0 {
		t.Errorf("sessions size = %d, want 0 (all sessions taken)", len(st.sessions))
	}
}
