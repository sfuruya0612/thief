package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/session"
)

// セッション系ハンドラのテストで使う仮のプロファイル・リージョン。実 AWS へ接続しないため、
// 値そのものは AWS の呼び出しにしか渡らない。
const (
	sessionTestProfile = "example-common"
	sessionTestRegion  = "ap-northeast-1"
)

// sessionStub は EC2 Session / ECS Exec の AWS 呼び出しを差し替えるスタブ。呼び出し回数と
// 引数を記録し、設定した結果を返す。データチャネルへの接続は必ず失敗させる。成功させると
// session.Bridge が実際の SSM のデータチャネルとして読み書きを始めてしまうためである。
type sessionStub struct {
	mu sync.Mutex

	startSSMCalls    int
	executeECSCalls  int
	openChannelCalls int
	terminateCalls   int

	// lastExecute は executeECSCommand に渡された (profile, region, cluster, task, container, command)。
	lastExecute [6]string
	// lastTerminateSessionID は terminateSSMSession に渡されたセッション ID。
	lastTerminateSessionID string

	// startResult / executeResult が nil のときは startErr / executeErr を返す。
	startResult   *awsinternal.StartSessionResult
	executeResult *awsinternal.StartSessionResult
	startErr      error
	executeErr    error
	openErr       error
}

func newSessionStub() *sessionStub {
	return &sessionStub{
		startErr:   errors.New("start ssm session failed"),
		executeErr: errors.New("execute ecs command failed"),
		openErr:    errors.New("open data channel failed"),
	}
}

func (st *sessionStub) startSSMSession(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.startSSMCalls++
	if st.startResult != nil {
		return st.startResult, nil
	}
	return nil, st.startErr
}

func (st *sessionStub) executeECSCommand(_ context.Context, profile, region, cluster, task, container, command string) (*awsinternal.StartSessionResult, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.executeECSCalls++
	st.lastExecute = [6]string{profile, region, cluster, task, container, command}
	if st.executeResult != nil {
		return st.executeResult, nil
	}
	return nil, st.executeErr
}

func (st *sessionStub) openDataChannel(context.Context, string, string, string) (*session.DataChannel, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.openChannelCalls++
	return nil, st.openErr
}

func (st *sessionStub) terminateSSMSession(_ context.Context, _, _, sessionID string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.terminateCalls++
	st.lastTerminateSessionID = sessionID
	return nil
}

// counts は (startSSMSession, executeECSCommand, openDataChannel) の呼び出し回数を返す。
func (st *sessionStub) counts() (int, int, int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.startSSMCalls, st.executeECSCalls, st.openChannelCalls
}

// terminated は terminateSSMSession の呼び出し回数と、最後に渡されたセッション ID を返す。
func (st *sessionStub) terminated() (int, string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.terminateCalls, st.lastTerminateSessionID
}

// executed は executeECSCommand に最後に渡された引数を返す。
func (st *sessionStub) executed() [6]string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.lastExecute
}

// newSessionTestServer はセッション系の AWS 呼び出しをスタブへ差し替えたテスト用 HTTP サーバを
// 起動する。httptest.Server の待ち受けアドレスを許可ホストに加える。newTestServer の
// allowedHosts は cfg.ListenAddr 由来で実際のポートと一致せず、hostCheckMiddleware が
// 421 を返してしまうためである。
func newSessionTestServer(t *testing.T, stub *sessionStub) *httptest.Server {
	t.Helper()

	s := newTestServer(t)
	// newTestServer は mux を組み立てないため、Handler を使うテストと同様に登録する。
	s.mux = http.NewServeMux()
	s.registerRoutes()
	s.startSSMSession = stub.startSSMSession
	s.executeECSCommand = stub.executeECSCommand
	s.openDataChannel = stub.openDataChannel
	s.terminateSSMSession = stub.terminateSSMSession

	ts := httptest.NewUnstartedServer(nil)
	s.allowedHosts = []string{ts.Listener.Addr().String()}
	ts.Config.Handler = s.Handler()
	ts.Start()
	t.Cleanup(ts.Close)
	return ts
}

func ec2SessionURL(baseURL string) string {
	return baseURL + "/api/aws/profiles/" + sessionTestProfile +
		"/ec2/i-0123456789abcdef0/session?region=" + sessionTestRegion
}

func ecsExecURL(baseURL, container string) string {
	u := baseURL + "/api/aws/profiles/" + sessionTestProfile +
		"/ecs/example-cluster/tasks/0123456789abcdef0123456789abcdef/exec?region=" + sessionTestRegion
	if container != "" {
		u += "&container=" + container
	}
	return u
}

// getSessionRoute はアップグレードのヘッダを付けない通常の GET を送り、応答のステータスを返す。
func getSessionRoute(t *testing.T, url string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// dialSessionRoute は WebSocket のアップグレードを行う。origin が空でなければ Origin を付ける
// (ブラウザの WebSocket は handshake に必ず Origin を付ける。RFC 6455 4.1)。返り値の conn は
// 成功時のみ非 nil。
func dialSessionRoute(t *testing.T, url, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	opts := &websocket.DialOptions{}
	if origin != "" {
		opts.HTTPHeader = http.Header{"Origin": []string{origin}}
	}
	return websocket.Dial(ctx, url, opts)
}

// sessionControlMessage はテスト側で制御メッセージの JSON を解釈するための型。
// 本体の型と独立に定義し、JSON の形 ({"type","message"}) を固定する。
type sessionControlMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// readSessionControlMessage はアップグレード後の WebSocket から TEXT の制御メッセージを 1 件読む。
func readSessionControlMessage(t *testing.T, conn *websocket.Conn) sessionControlMessage {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read control message: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("message type = %v, want %v", typ, websocket.MessageText)
	}
	var msg sessionControlMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal control message %q: %v", data, err)
	}
	return msg
}

// expectSetupFailureClose は、エラーのメッセージの後にサーバが WebSocket をステータス 1011
// (websocket.StatusInternalError) で閉じることを確認する。
func expectSetupFailureClose(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err == nil {
		t.Fatalf("read after error message returned data %q, want close", data)
	}
	if got := websocket.CloseStatus(err); got != websocket.StatusInternalError {
		t.Errorf("close status = %v (err %v), want %v", got, err, websocket.StatusInternalError)
	}
}

// TestEC2SessionRejectsBeforeAWS は、WebSocket のアップグレードでないリクエストと、許可されない
// Origin のアップグレードが、AWS のセッションを開始する前に拒否されることを確認する。
func TestEC2SessionRejectsBeforeAWS(t *testing.T) {
	captureLogs(t)

	tests := []struct {
		name       string
		dial       bool
		origin     string
		wantStatus int
	}{
		{
			// <img src="..."> のようなアップグレードのヘッダを持たない GET。
			name:       "plain GET without upgrade headers",
			wantStatus: http.StatusUpgradeRequired,
		},
		{
			name:       "upgrade with a disallowed origin",
			dial:       true,
			origin:     "https://evil.example",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "upgrade with a null origin",
			dial:       true,
			origin:     "null",
			wantStatus: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newSessionStub()
			ts := newSessionTestServer(t, stub)
			url := ec2SessionURL(ts.URL)

			var status int
			if tt.dial {
				conn, resp, err := dialSessionRoute(t, url, tt.origin)
				if conn != nil {
					_ = conn.Close(websocket.StatusNormalClosure, "")
				}
				if err == nil {
					t.Fatalf("dial succeeded, want rejection")
				}
				if resp == nil {
					t.Fatalf("dial: %v (no response)", err)
				}
				status = resp.StatusCode
			} else {
				status = getSessionRoute(t, url)
			}

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if starts, execs, opens := stub.counts(); starts != 0 || execs != 0 || opens != 0 {
				t.Errorf("AWS calls = (start %d, execute %d, open data channel %d), want all 0",
					starts, execs, opens)
			}
		})
	}
}

// TestECSExecRejectsBeforeAWS は EC2 Session と同じ拒否に加え、container の無いリクエストが
// アップグレードより前に 400 で拒否されることを確認する。
func TestECSExecRejectsBeforeAWS(t *testing.T) {
	captureLogs(t)

	tests := []struct {
		name       string
		dial       bool
		origin     string
		container  string
		wantStatus int
	}{
		{
			name:       "missing container",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "plain GET without upgrade headers",
			container:  "app",
			wantStatus: http.StatusUpgradeRequired,
		},
		{
			name:       "upgrade with a disallowed origin",
			dial:       true,
			origin:     "https://evil.example",
			container:  "app",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "upgrade with a null origin",
			dial:       true,
			origin:     "null",
			container:  "app",
			wantStatus: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newSessionStub()
			ts := newSessionTestServer(t, stub)
			url := ecsExecURL(ts.URL, tt.container)

			var status int
			if tt.dial {
				conn, resp, err := dialSessionRoute(t, url, tt.origin)
				if conn != nil {
					_ = conn.Close(websocket.StatusNormalClosure, "")
				}
				if err == nil {
					t.Fatalf("dial succeeded, want rejection")
				}
				if resp == nil {
					t.Fatalf("dial: %v (no response)", err)
				}
				status = resp.StatusCode
			} else {
				status = getSessionRoute(t, url)
			}

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if starts, execs, opens := stub.counts(); starts != 0 || execs != 0 || opens != 0 {
				t.Errorf("AWS calls = (start %d, execute %d, open data channel %d), want all 0",
					starts, execs, opens)
			}
		})
	}
}

// TestEC2SessionAcceptsAllowedOrigin は、許可する Origin (frontend の dev server の既定の 2 つ) と
// Origin の無いアップグレード (ブラウザ以外のクライアント) がアップグレードを通過し、AWS の
// セッションを開始することを確認する。セッションの開始に失敗した場合は、ブラウザが原因の
// メッセージを TEXT で受け取り、WebSocket が 1011 で閉じる。セッションは開始していないため、
// 終了処理は呼ばれない。
func TestEC2SessionAcceptsAllowedOrigin(t *testing.T) {
	captureLogs(t)

	tests := []struct {
		name   string
		origin string
	}{
		{name: "allowed origin localhost", origin: "http://localhost:8088"},
		{name: "allowed origin 127.0.0.1", origin: "http://127.0.0.1:8088"},
		{name: "no origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newSessionStub()
			stub.startErr = errors.New("start ssm session failed: boom")
			ts := newSessionTestServer(t, stub)

			conn, resp, err := dialSessionRoute(t, ec2SessionURL(ts.URL), tt.origin)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
			}

			msg := readSessionControlMessage(t, conn)
			if msg.Type != "error" {
				t.Errorf("message type = %q, want %q", msg.Type, "error")
			}
			if msg.Message != "start ssm session failed: boom" {
				t.Errorf("message = %q, want %q", msg.Message, "start ssm session failed: boom")
			}
			expectSetupFailureClose(t, conn)
			if starts, execs, opens := stub.counts(); starts != 1 || execs != 0 || opens != 0 {
				t.Errorf("AWS calls = (start %d, execute %d, open data channel %d), want (1, 0, 0)",
					starts, execs, opens)
			}
			if n, _ := stub.terminated(); n != 0 {
				t.Errorf("terminate calls = %d, want 0", n)
			}
		})
	}
}

// TestECSExecAcceptsAllowedOrigin は ECS Exec でも同じく、許可する Origin のアップグレードで
// ExecuteCommand が呼ばれ、失敗時にブラウザが原因のメッセージを受け取ることを確認する。
// ExecuteCommand に渡る引数 (command の未指定時の既定値 /bin/sh を含む) も確認する。
func TestECSExecAcceptsAllowedOrigin(t *testing.T) {
	captureLogs(t)

	const (
		cluster = "example-cluster"
		task    = "0123456789abcdef0123456789abcdef"
	)
	tests := []struct {
		name        string
		origin      string
		query       string
		wantCommand string
	}{
		{name: "allowed origin localhost with default command", origin: "http://localhost:8088", wantCommand: ecsExecDefaultCommand},
		{name: "allowed origin 127.0.0.1 with explicit command", origin: "http://127.0.0.1:8088", query: "&command=ls", wantCommand: "ls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newSessionStub()
			stub.executeErr = errors.New("execute ecs command failed: boom")
			ts := newSessionTestServer(t, stub)

			conn, resp, err := dialSessionRoute(t, ecsExecURL(ts.URL, "app")+tt.query, tt.origin)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
			}

			msg := readSessionControlMessage(t, conn)
			if msg.Type != "error" {
				t.Errorf("message type = %q, want %q", msg.Type, "error")
			}
			if msg.Message != "execute ecs command failed: boom" {
				t.Errorf("message = %q, want %q", msg.Message, "execute ecs command failed: boom")
			}
			expectSetupFailureClose(t, conn)
			if starts, execs, opens := stub.counts(); starts != 0 || execs != 1 || opens != 0 {
				t.Errorf("AWS calls = (start %d, execute %d, open data channel %d), want (0, 1, 0)",
					starts, execs, opens)
			}
			want := [6]string{sessionTestProfile, sessionTestRegion, cluster, task, "app", tt.wantCommand}
			if got := stub.executed(); got != want {
				t.Errorf("execute ecs command args = %q, want %q", got, want)
			}
			if n, _ := stub.terminated(); n != 0 {
				t.Errorf("terminate calls = %d, want 0", n)
			}
		})
	}
}

// TestSessionDataChannelFailureTerminates は、AWS のセッションの開始に成功した後にデータチャネルへの
// 接続が失敗した場合、開始したセッションを終了し、ブラウザが原因のメッセージを受け取り、WebSocket が
// 1011 で閉じることを確認する。
func TestSessionDataChannelFailureTerminates(t *testing.T) {
	captureLogs(t)

	result := &awsinternal.StartSessionResult{
		SessionID:  "example-session-0217",
		StreamURL:  "wss://ssmmessages.example.invalid/v1/data-channel/example-session-0217",
		TokenValue: "example-token",
	}
	tests := []struct {
		name      string
		url       func(baseURL string) string
		wantStart int
		wantExec  int
	}{
		{name: "ec2 session", url: ec2SessionURL, wantStart: 1},
		{name: "ecs exec", url: func(baseURL string) string { return ecsExecURL(baseURL, "app") }, wantExec: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newSessionStub()
			stub.startResult = result
			stub.executeResult = result
			ts := newSessionTestServer(t, stub)

			conn, resp, err := dialSessionRoute(t, tt.url(ts.URL), "http://localhost:8088")
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
			}

			msg := readSessionControlMessage(t, conn)
			if msg.Type != "error" {
				t.Errorf("message type = %q, want %q", msg.Type, "error")
			}
			if want := "open data channel: open data channel failed"; msg.Message != want {
				t.Errorf("message = %q, want %q", msg.Message, want)
			}
			expectSetupFailureClose(t, conn)
			if starts, execs, opens := stub.counts(); starts != tt.wantStart || execs != tt.wantExec || opens != 1 {
				t.Errorf("AWS calls = (start %d, execute %d, open data channel %d), want (%d, %d, 1)",
					starts, execs, opens, tt.wantStart, tt.wantExec)
			}
			// terminateBestEffort はエラーのメッセージの送信より前に呼ばれるため、受信の後は確定している。
			if n, id := stub.terminated(); n != 1 || id != result.SessionID {
				t.Errorf("terminate = (%d calls, session %q), want (1, %q)", n, id, result.SessionID)
			}
		})
	}
}
