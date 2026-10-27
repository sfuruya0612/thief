package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/session"
	"github.com/sfuruya0612/thief/backend/internal/tui"
)

// TestNewTUIDeps_ServicesFollowWebUIOrder は TUI に載せるプロバイダーとサービスの集合、並びを
// 固定する。並びは Web UI のトップバー (frontend/src/components/TopBar.tsx) とサイドバー
// (frontend/src/lib/serviceMeta.ts) と同じにする。サービスを足したら、ここの期待値も更新する。
func TestNewTUIDeps_ServicesFollowWebUIOrder(t *testing.T) {
	deps := newTUIDeps(config.Defaults())

	var providers []string
	for _, p := range deps.Providers {
		providers = append(providers, p.ID())
	}
	if want := []string{tui.ProviderAWS, tui.ProviderGCP, tui.ProviderDatadog, tui.ProviderTiDB}; !reflect.DeepEqual(providers, want) {
		t.Errorf("provider IDs = %v, want %v", providers, want)
	}

	wantServices := map[string][]string{
		tui.ProviderAWS:     {"ec2", "lambda", "ecr", "ecs", "s3", "rds", "dynamo", "cache", "elb", "cloudfront", "apigw", "natgw", "kinesis", "sqs", "iam", "waf", "secrets", "ssm", "cfn"},
		tui.ProviderGCP:     {"cloudrun", "gcs", "gcpiam", "gcpserviceaccounts"},
		tui.ProviderDatadog: {"datadog-dashboards", "datadog-cost-historical", "datadog-cost-estimated"},
		tui.ProviderTiDB:    {"tidb-projects", "tidb-cost"},
	}
	for _, p := range deps.Providers {
		var got []string
		seen := map[string]bool{}
		for _, s := range p.Services() {
			if seen[s.ID()] {
				t.Errorf("provider %s: service %q is registered twice", p.ID(), s.ID())
			}
			seen[s.ID()] = true
			got = append(got, s.ID())
		}
		if !reflect.DeepEqual(got, wantServices[p.ID()]) {
			t.Errorf("%s service IDs = %v, want %v", p.ID(), got, wantServices[p.ID()])
		}
	}

	if len(deps.Providers) == 0 || deps.OpenEC2Session == nil || deps.OpenECSExecSession == nil {
		t.Errorf("deps must wire Providers, OpenEC2Session and OpenECSExecSession: %+v", deps)
	}
	// 同じシグネチャの別の関数を配線しても非 nil の確認では通るため、関数の同一性で固定する。
	wired := map[string][2]uintptr{
		"OpenEC2Session":     {reflect.ValueOf(deps.OpenEC2Session).Pointer(), reflect.ValueOf(openTUIEC2Session).Pointer()},
		"OpenECSExecSession": {reflect.ValueOf(deps.OpenECSExecSession).Pointer(), reflect.ValueOf(openTUIECSExecSession).Pointer()},
		"ListECSTasks":       {reflect.ValueOf(deps.ListECSTasks).Pointer(), reflect.ValueOf(awsinternal.ListECSTasks).Pointer()},
		"ListECSContainers":  {reflect.ValueOf(deps.ListECSContainers).Pointer(), reflect.ValueOf(awsinternal.ListECSContainers).Pointer()},
	}
	for name, pair := range wired {
		if pair[0] == 0 || pair[0] != pair[1] {
			t.Errorf("deps.%s must be the production function", name)
		}
	}
}

func TestDefaultTUIAdapterDeps_UsesDataChannel(t *testing.T) {
	deps := defaultTUIAdapterDeps()
	wired := map[string][2]uintptr{
		"startSession":     {reflect.ValueOf(deps.startSession).Pointer(), reflect.ValueOf(awsinternal.StartSSMSession).Pointer()},
		"executeSession":   {reflect.ValueOf(deps.executeSession).Pointer(), reflect.ValueOf(awsinternal.ExecuteECSCommand).Pointer()},
		"openDataChannel":  {reflect.ValueOf(deps.openDataChannel).Pointer(), reflect.ValueOf(openTUIDataChannel).Pointer()},
		"terminateSession": {reflect.ValueOf(deps.terminateSession).Pointer(), reflect.ValueOf(awsinternal.TerminateSSMSession).Pointer()},
	}
	for name, pair := range wired {
		if pair[0] == 0 || pair[0] != pair[1] {
			t.Errorf("%s is not wired to the production DataChannel function", name)
		}
	}
}

func TestNewTUIDeps_TabsFollowWebUI(t *testing.T) {
	want := map[string][]string{
		"ec2":        {"Overview", "Terminal", "Tags"},
		"lambda":     {"Overview", "Tags"},
		"ecr":        {"Overview", "Images"},
		"ecs":        {"Overview", "Services", "Tasks", "Instances", "Terminal", "Tags"},
		"s3":         {"Overview", "Objects", "Tags"},
		"rds":        {"Overview", "Instance Parameters", "Cluster Parameters", "Tags"},
		"dynamo":     {"Overview", "Items", "Tags"},
		"cache":      {"Overview", "Parameters", "Tags"},
		"elb":        {"Overview", "Listeners", "Targets", "Tags"},
		"cloudfront": {"Overview", "Behaviors", "Tags"},
		"apigw":      {"Overview", "Tags"},
		"natgw":      {"Overview", "Tags"},
		"kinesis":    {"Overview", "Tags"},
		"sqs":        {"Overview", "Tags"},
		"iam":        {"Overview", "Tags"},
		"waf":        {"Overview", "Rules", "Tags"},
		"secrets":    {"Overview", "Value", "Tags"},
		"ssm":        {"Overview", "Value", "Tags"},
		"cfn":        {"Overview", "Events", "Resources", "Tags"},

		"cloudrun":                {"Overview"},
		"gcs":                     {"Overview", "Objects"},
		"gcpiam":                  {"Overview"},
		"gcpserviceaccounts":      {"Overview"},
		"datadog-dashboards":      {"Overview", "Widgets"},
		"datadog-cost-historical": {"Overview"},
		"datadog-cost-estimated":  {"Overview"},
		"tidb-projects":           {"Overview", "Clusters"},
		"tidb-cost":               {"Overview"},
	}
	deps := newTUIDeps(config.Defaults())
	for _, p := range deps.Providers {
		for _, svc := range p.Services() {
			got := []string{"Overview", "Tags"}
			if provider, ok := svc.(tui.TabProvider); ok {
				got = nil
				for _, tab := range provider.Tabs() {
					got = append(got, tab.Title)
				}
			}
			if !reflect.DeepEqual(got, want[svc.ID()]) {
				t.Errorf("%s tabs = %v, want %v", svc.ID(), got, want[svc.ID()])
			}
		}
	}
}

// fakeTUIChannel は tuiDataChannel の偽物。
type fakeTUIChannel struct {
	readResult session.ReadResult
	readErr    error

	inputs []struct {
		payloadType session.PayloadType
		data        []byte
	}
	sizes    [][2]uint32
	closeRun int
	closeErr error
}

func (c *fakeTUIChannel) Read(context.Context) (session.ReadResult, error) {
	return c.readResult, c.readErr
}

func (c *fakeTUIChannel) SendInput(_ context.Context, payloadType session.PayloadType, data []byte) error {
	c.inputs = append(c.inputs, struct {
		payloadType session.PayloadType
		data        []byte
	}{payloadType: payloadType, data: append([]byte(nil), data...)})
	return nil
}

func (c *fakeTUIChannel) SendSize(_ context.Context, cols, rows uint32) error {
	c.sizes = append(c.sizes, [2]uint32{cols, rows})
	return nil
}

func (c *fakeTUIChannel) Close() error {
	c.closeRun++
	return c.closeErr
}

type tuiSessionCalls struct {
	startTargets     []string
	executeArgs      []string
	openArgs         []string
	terminateIDs     []string
	terminateProfile string
	terminateRegion  string
}

func TestOpenTUIEC2SessionWith_Success(t *testing.T) {
	rec := &tuiSessionCalls{}
	channel := &fakeTUIChannel{
		readResult: session.ReadResult{Output: []byte("ok"), Closed: true, CloseMessage: "bye"},
	}
	deps := tuiAdapterDeps{
		startSession: func(_ context.Context, profile, region, target string) (*awsinternal.StartSessionResult, error) {
			rec.startTargets = append(rec.startTargets, profile+"/"+region+"/"+target)
			return &awsinternal.StartSessionResult{SessionID: "sess-1", StreamURL: "wss://stream", TokenValue: "token-1"}, nil
		},
		openDataChannel: func(_ context.Context, streamURL, tokenValue, sessionID string) (tuiDataChannel, error) {
			rec.openArgs = append(rec.openArgs, streamURL+"/"+tokenValue+"/"+sessionID)
			return channel, nil
		},
		terminateSession: func(ctx context.Context, profile, region, sessionID string) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("terminate context must have a deadline")
			}
			rec.terminateIDs = append(rec.terminateIDs, sessionID)
			rec.terminateProfile, rec.terminateRegion = profile, region
			return nil
		},
	}

	got, err := openTUIEC2SessionWith(context.Background(), deps, "prof", "ap-northeast-1", "i-web")
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if want := []string{"prof/ap-northeast-1/i-web"}; !reflect.DeepEqual(rec.startTargets, want) {
		t.Errorf("start targets = %v, want %v", rec.startTargets, want)
	}
	if want := []string{"wss://stream/token-1/sess-1"}; !reflect.DeepEqual(rec.openArgs, want) {
		t.Errorf("open args = %v, want %v", rec.openArgs, want)
	}

	output, err := got.Read(context.Background())
	if err != nil || string(output.Data) != "ok" || !output.Closed || output.Message != "bye" {
		t.Errorf("Read = %+v, %v", output, err)
	}
	if err := got.SendInput(context.Background(), []byte("x")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}
	if len(channel.inputs) != 1 || channel.inputs[0].payloadType != session.PayloadTypeOutput || string(channel.inputs[0].data) != "x" {
		t.Errorf("inputs = %+v", channel.inputs)
	}
	if err := got.SendSize(context.Background(), 80, 24); err != nil {
		t.Fatalf("SendSize: %v", err)
	}
	if len(channel.sizes) != 1 || channel.sizes[0] != [2]uint32{80, 24} {
		t.Errorf("sizes = %v", channel.sizes)
	}

	if err := got.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if channel.closeRun != 1 || len(rec.terminateIDs) != 1 || rec.terminateIDs[0] != "sess-1" {
		t.Errorf("close calls = %d, terminate = %v", channel.closeRun, rec.terminateIDs)
	}
	if rec.terminateProfile != "prof" || rec.terminateRegion != "ap-northeast-1" {
		t.Errorf("terminate profile/region = %q/%q", rec.terminateProfile, rec.terminateRegion)
	}
	// Close は冪等で、2 回目はデータチャネルも Terminate も呼ばない。
	if err := got.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if channel.closeRun != 1 || len(rec.terminateIDs) != 1 {
		t.Errorf("second Close must be idempotent: channel=%d terminate=%v", channel.closeRun, rec.terminateIDs)
	}
}

func TestTUISessionCloseUsesDetachedContext(t *testing.T) {
	channel := &fakeTUIChannel{}
	deps := tuiAdapterDeps{
		startSession: func(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
			return &awsinternal.StartSessionResult{SessionID: "sess-1", StreamURL: "wss://stream", TokenValue: "token-1"}, nil
		},
		openDataChannel: func(context.Context, string, string, string) (tuiDataChannel, error) {
			return channel, nil
		},
		terminateSession: func(ctx context.Context, _, _, _ string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		},
	}
	got, err := openTUIEC2SessionWith(context.Background(), deps, "prof", "ap-northeast-1", "i-web")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := got.Close(ctx); err != nil {
		t.Errorf("Close with canceled context = %v, want nil", err)
	}
}

func TestOpenTUIEC2SessionWith_OpenDataChannelFailureTerminates(t *testing.T) {
	rec := &tuiSessionCalls{}
	deps := tuiAdapterDeps{
		startSession: func(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
			return &awsinternal.StartSessionResult{SessionID: "sess-1", StreamURL: "wss://stream", TokenValue: "token-1"}, nil
		},
		openDataChannel: func(context.Context, string, string, string) (tuiDataChannel, error) {
			return nil, errors.New("dial failed")
		},
		terminateSession: func(ctx context.Context, _, _, sessionID string) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("cleanup context must have a deadline")
			}
			rec.terminateIDs = append(rec.terminateIDs, sessionID)
			return errors.New("terminate failed")
		},
	}

	_, err := openTUIEC2SessionWith(context.Background(), deps, "prof", "ap-northeast-1", "i-web")
	if err == nil {
		t.Fatal("error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "terminate session: terminate failed") {
		t.Errorf("error = %q, want both open and terminate failures", err)
	}
	var startErr *tui.SessionStartError
	if !errors.As(err, &startErr) || !startErr.Started {
		t.Errorf("error = %#v, want started SessionStartError", err)
	}
	if !strings.Contains(err.Error(), "open data channel: dial failed") {
		t.Errorf("error = %q", err)
	}
	if !reflect.DeepEqual(rec.terminateIDs, []string{"sess-1"}) {
		t.Errorf("terminate = %v, want [sess-1]", rec.terminateIDs)
	}
}

func TestOpenTUIEC2SessionWith_StartFailureDoesNotTerminate(t *testing.T) {
	rec := &tuiSessionCalls{}
	deps := tuiAdapterDeps{
		startSession: func(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
			return nil, errors.New("AccessDeniedException")
		},
		openDataChannel: func(context.Context, string, string, string) (tuiDataChannel, error) {
			t.Fatal("OpenDataChannel must not run when StartSSMSession fails")
			return nil, nil
		},
		terminateSession: func(context.Context, string, string, string) error {
			rec.terminateIDs = append(rec.terminateIDs, "unexpected")
			return nil
		},
	}

	_, err := openTUIEC2SessionWith(context.Background(), deps, "prof", "ap-northeast-1", "i-web")
	if err == nil || !strings.Contains(err.Error(), "start session: AccessDeniedException") {
		t.Fatalf("error = %v", err)
	}
	var startErr *tui.SessionStartError
	if !errors.As(err, &startErr) || startErr.Started {
		t.Errorf("error = %#v, want not-started SessionStartError", err)
	}
	if len(rec.terminateIDs) != 0 {
		t.Errorf("terminate = %v, want none", rec.terminateIDs)
	}
}

func TestOpenTUIECSExecSessionWith_Success(t *testing.T) {
	rec := &tuiSessionCalls{}
	channel := &fakeTUIChannel{}
	deps := tuiAdapterDeps{
		executeSession: func(_ context.Context, profile, region, cluster, task, container, command string) (*awsinternal.StartSessionResult, error) {
			rec.executeArgs = append(rec.executeArgs, strings.Join([]string{profile, region, cluster, task, container, command}, "/"))
			return &awsinternal.StartSessionResult{SessionID: "sess-1", StreamURL: "wss://stream", TokenValue: "token-1"}, nil
		},
		openDataChannel: func(context.Context, string, string, string) (tuiDataChannel, error) {
			return channel, nil
		},
		terminateSession: func(_ context.Context, _, _, sessionID string) error {
			rec.terminateIDs = append(rec.terminateIDs, sessionID)
			return nil
		},
	}

	got, err := openTUIECSExecSessionWith(context.Background(), deps, "prof", "ap-northeast-1", "demo", "task-a", "app", "/bin/sh")
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	want := []string{"prof/ap-northeast-1/demo/task-a/app//bin/sh"}
	if !reflect.DeepEqual(rec.executeArgs, want) {
		t.Errorf("ExecuteECSCommand args = %v, want %v", rec.executeArgs, want)
	}
	if err := got.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if channel.closeRun != 1 || !reflect.DeepEqual(rec.terminateIDs, []string{"sess-1"}) {
		t.Errorf("close=%d terminate=%v", channel.closeRun, rec.terminateIDs)
	}
}

func TestOpenTUIEC2SessionWith_NoPluginLookup(t *testing.T) {
	// PATH に session-manager-plugin が無くても、TUI のアダプタはプラグインを解決しない。
	t.Setenv("PATH", "")
	channel := &fakeTUIChannel{}
	deps := tuiAdapterDeps{
		startSession: func(context.Context, string, string, string) (*awsinternal.StartSessionResult, error) {
			return &awsinternal.StartSessionResult{SessionID: "sess-1", StreamURL: "wss://stream", TokenValue: "token-1"}, nil
		},
		openDataChannel: func(context.Context, string, string, string) (tuiDataChannel, error) {
			return channel, nil
		},
		terminateSession: func(context.Context, string, string, string) error { return nil },
	}
	got, err := openTUIEC2SessionWith(context.Background(), deps, "prof", "ap-northeast-1", "i-web")
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if err := got.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpenTUIECSExecSessionWith_OpenDataChannelFailureTerminates(t *testing.T) {
	rec := &tuiSessionCalls{}
	deps := tuiAdapterDeps{
		executeSession: func(context.Context, string, string, string, string, string, string) (*awsinternal.StartSessionResult, error) {
			return &awsinternal.StartSessionResult{SessionID: "sess-1", StreamURL: "wss://stream", TokenValue: "token-1"}, nil
		},
		openDataChannel: func(context.Context, string, string, string) (tuiDataChannel, error) {
			return nil, errors.New("dial failed")
		},
		terminateSession: func(_ context.Context, _, _, sessionID string) error {
			rec.terminateIDs = append(rec.terminateIDs, sessionID)
			return nil
		},
	}

	_, err := openTUIECSExecSessionWith(context.Background(), deps, "prof", "ap-northeast-1", "demo", "task-a", "app", "/bin/sh")
	if err == nil || !strings.Contains(err.Error(), "open data channel: dial failed") {
		t.Fatalf("error = %v", err)
	}
	var startErr *tui.SessionStartError
	if !errors.As(err, &startErr) || !startErr.Started {
		t.Errorf("error = %#v, want started SessionStartError", err)
	}
	if !reflect.DeepEqual(rec.terminateIDs, []string{"sess-1"}) {
		t.Errorf("terminate = %v, want [sess-1]", rec.terminateIDs)
	}
}
