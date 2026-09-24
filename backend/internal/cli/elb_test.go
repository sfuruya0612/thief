package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/spf13/cobra"
)

// TestNewELBCmd はコマンド木の形を確かめる。
//
// `thief elb` 単体は従来どおりロードバランサの一覧を出す。サブコマンドを足したことで
// 親の本体が失われると、既存の使い方が動かなくなる。
func TestNewELBCmd(t *testing.T) {
	elbCmd := newELBCmd()
	if elbCmd.Name() != "elb" {
		t.Errorf("command name = %q, want %q", elbCmd.Name(), "elb")
	}
	if elbCmd.RunE == nil {
		t.Error("elb command has no RunE; `thief elb` can no longer list load balancers")
	}

	var names []string
	for _, c := range elbCmd.Commands() {
		names = append(names, c.Name())
	}
	want := []string{"listeners", "rules", "target-groups", "target-health"}
	if diff := cmp.Diff(want, names); diff != "" {
		t.Errorf("subcommands mismatch (-want +got):\n%s", diff)
	}
}

// TestELBSubcommandsRequireTargetFlag は 4 つのサブコマンドが対象の指定を必須に
// していることを、対象を省略した実行が AWS へ接続する前のフラグ検証で止まることで
// 確かめる。
func TestELBSubcommandsRequireTargetFlag(t *testing.T) {
	// loadConfig の先の config.Load が実行環境の設定を読まないよう、参照先を空に向ける。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	tests := []struct {
		name     string
		args     []string
		wantFlag string
	}{
		{name: "listeners requires --lb", args: []string{"elb", "listeners"}, wantFlag: "lb"},
		{name: "rules requires --listener", args: []string{"elb", "rules"}, wantFlag: "listener"},
		{name: "target-groups requires --lb", args: []string{"elb", "target-groups"}, wantFlag: "lb"},
		{name: "target-health requires --target-group", args: []string{"elb", "target-health"}, wantFlag: "target-group"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewRootCmd()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tt.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want the required flag error")
			}
			want := fmt.Sprintf("required flag(s) %q not set", tt.wantFlag)
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), want)
			}
		})
	}
}

// TestResolveELBLoadBalancerArn は --lb の値の解決を確かめる。
//
// ARN は一覧を引かずにそのまま使い、名前は一覧の Name と突き合わせて ID (ARN) を返す。
func TestResolveELBLoadBalancerArn(t *testing.T) {
	listErr := errors.New("boom")
	lbArn := "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/lb-1/abc"

	tests := []struct {
		name string
		// value は --lb に渡された値。
		value string
		// lbs と listErr は一覧の取得の結果。
		lbs     []awsinternal.ELBResource
		listErr error
		// want は解決結果の ARN。エラーの場合は空。
		want string
		// wantErrContains はエラー文言に含まれてほしい断片。
		wantErrContains string
		// wantErrIs は errors.Is で辿れるべき元のエラー。
		wantErrIs error
		// wantListCalls は一覧の取得が呼ばれた回数。
		wantListCalls int
	}{
		{
			name:          "arn is used as is",
			value:         lbArn,
			want:          lbArn,
			wantListCalls: 0,
		},
		{
			name:  "name resolves to the arn",
			value: "lb-1",
			lbs: []awsinternal.ELBResource{
				{Name: "lb-0", ID: "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/lb-0/abc"},
				{Name: "lb-1", ID: lbArn},
			},
			want:          lbArn,
			wantListCalls: 1,
		},
		{
			name:            "unknown name",
			value:           "lb-9",
			lbs:             []awsinternal.ELBResource{{Name: "lb-1", ID: lbArn}},
			wantErrContains: "load balancer not found: lb-9",
			wantListCalls:   1,
		},
		{
			name:            "empty value",
			value:           "",
			wantErrContains: "load balancer name or ARN is required",
			wantListCalls:   0,
		},
		{
			name:            "list error",
			value:           "lb-1",
			listErr:         listErr,
			wantErrContains: "list load balancers: boom",
			wantErrIs:       listErr,
			wantListCalls:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var gotProfile, gotRegion string
			lister := func(_ context.Context, profile, region string) ([]awsinternal.ELBResource, error) {
				calls++
				gotProfile = profile
				gotRegion = region
				return tt.lbs, tt.listErr
			}

			got, err := resolveELBLoadBalancerArn(
				context.Background(), lister, "test-profile", "ap-northeast-1", tt.value,
			)

			if tt.wantErrContains == "" && tt.wantErrIs == nil {
				if err != nil {
					t.Fatalf("resolveELBLoadBalancerArn() error = %v, want nil", err)
				}
				if got != tt.want {
					t.Errorf("resolveELBLoadBalancerArn() = %q, want %q", got, tt.want)
				}
			} else {
				if err == nil {
					t.Fatalf("resolveELBLoadBalancerArn() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrContains)
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("errors.Is() = false, want true; the chain is severed: %v", err)
				}
			}

			if calls != tt.wantListCalls {
				t.Errorf("lister called %d times, want %d", calls, tt.wantListCalls)
			}
			// 解決は profile と region を取得関数へそのまま渡す。取り違えると
			// 別アカウント・別リージョンの一覧を引く。
			if calls > 0 && (gotProfile != "test-profile" || gotRegion != "ap-northeast-1") {
				t.Errorf("lister called with profile %q region %q, want %q and %q",
					gotProfile, gotRegion, "test-profile", "ap-northeast-1")
			}
		})
	}
}

// TestELBRejectsUnknownSubcommand は綴りを誤ったサブコマンド名がロードバランサの一覧の
// 出力に化けず、エラーになることを確かめる。
//
// 親の elb は RunE を持つため、位置引数を受け付けると cobra は未知の先頭引数を
// そのまま親の RunE に渡してしまう。
func TestELBRejectsUnknownSubcommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"elb", "listener"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want an unknown command error")
	}
	if !strings.Contains(err.Error(), `unknown command "listener"`) {
		t.Errorf("error = %q, want it to mention the unknown command", err.Error())
	}
}

// elbOpsCall は fakeELBOps が記録する呼び出しの内容。arg は対象 (ロードバランサ、
// リスナー、ターゲットグループ) の ARN。
type elbOpsCall struct {
	op      string
	profile string
	region  string
	arg     string
}

// fakeELBOps はサブコマンドの結線を検証するための取得関数の集合を返す。各関数は
// 呼び出しを calls に記録し、与えた結果をそのまま返す。
func fakeELBOps(calls *[]elbOpsCall, lbs []awsinternal.ELBResource) elbOps {
	return elbOps{
		listLoadBalancers: func(_ context.Context, profile, region string) ([]awsinternal.ELBResource, error) {
			*calls = append(*calls, elbOpsCall{op: "listLoadBalancers", profile: profile, region: region})
			return lbs, nil
		},
		listListeners: func(_ context.Context, profile, region, lbArn string) ([]awsinternal.ELBListenerResource, error) {
			*calls = append(*calls, elbOpsCall{op: "listListeners", profile: profile, region: region, arg: lbArn})
			return []awsinternal.ELBListenerResource{{
				ARN:                   "arn:listener-1",
				Protocol:              "HTTPS",
				Port:                  443,
				DefaultActionType:     "forward",
				DefaultTargetGroupArn: "arn:tg-1",
			}}, nil
		},
		listRules: func(_ context.Context, profile, region, listenerArn string) ([]awsinternal.ELBRuleResource, error) {
			*calls = append(*calls, elbOpsCall{op: "listRules", profile: profile, region: region, arg: listenerArn})
			// 1 条件の中に "," を含む値で、CSV では引用され、tab では列が崩れないことを見る。
			return []awsinternal.ELBRuleResource{{
				ARN:            "arn:rule-1",
				Priority:       "10",
				Conditions:     []string{"host-header=a.example.com,b.example.com", "path-pattern=/api/*"},
				ActionType:     "forward",
				TargetGroupArn: "arn:tg-1",
			}}, nil
		},
		listTargetGroups: func(_ context.Context, profile, region, lbArn string) ([]awsinternal.ELBTargetGroupResource, error) {
			*calls = append(*calls, elbOpsCall{op: "listTargetGroups", profile: profile, region: region, arg: lbArn})
			return []awsinternal.ELBTargetGroupResource{{
				ARN:             "arn:tg-1",
				Name:            "tg-1",
				Protocol:        "HTTP",
				Port:            8080,
				TargetType:      "ip",
				VpcID:           "vpc-1",
				HealthCheckPath: "/health",
			}}, nil
		},
		describeTargetHealth: func(_ context.Context, profile, region, tgArn string) ([]awsinternal.ELBTargetHealthResource, error) {
			*calls = append(*calls, elbOpsCall{op: "describeTargetHealth", profile: profile, region: region, arg: tgArn})
			return []awsinternal.ELBTargetHealthResource{{
				TargetID:         "10.0.1.10",
				Port:             8080,
				AvailabilityZone: "ap-northeast-1a",
				State:            "unhealthy",
				Reason:           "Target.FailedHealthChecks",
				Description:      "Health checks failed",
			}}, nil
		},
	}
}

// newRootCmdWithELBOps は実コマンドツリーの elb を、取得関数を差し替えた elb に入れ替えた
// root を返す。root の永続フラグ (-p / -r / -o) を含めて実行できる。
func newRootCmdWithELBOps(t *testing.T, ops elbOps) *cobra.Command {
	t.Helper()
	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.Name() == "elb" {
			root.RemoveCommand(c)
		}
	}
	root.AddCommand(newELBCmdWithOps(ops))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

// TestDefaultELBOpsWired は本番の取得関数の束が 5 つとも埋まっていることを見る。nil の
// フィールドはコンパイルでは検知できず、実行時に対応するコマンドが落ちる。
func TestDefaultELBOpsWired(t *testing.T) {
	ops := defaultELBOps()
	if ops.listLoadBalancers == nil {
		t.Error("listLoadBalancers is nil")
	}
	if ops.listListeners == nil {
		t.Error("listListeners is nil")
	}
	if ops.listRules == nil {
		t.Error("listRules is nil")
	}
	if ops.listTargetGroups == nil {
		t.Error("listTargetGroups is nil")
	}
	if ops.describeTargetHealth == nil {
		t.Error("describeTargetHealth is nil")
	}
}

// TestELBSubcommandOutput は親の elb と 4 つのサブコマンドについて、フラグの値が取得関数の
// 引数へ届き、その結果が列定義どおりに tab / CSV で出力されることを、実コマンドツリーの
// 実行で確かめる。
//
// --lb は名前なら一覧を引いて ARN へ解決し、ARN ならそのまま渡す。
func TestELBSubcommandOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	lbArn := "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/lb-1/abc"
	lbs := []awsinternal.ELBResource{{
		Name:    "lb-1",
		ID:      lbArn,
		Type:    "application",
		State:   "active",
		Scheme:  "internet-facing",
		DNSName: "lb-1.example.com",
		VpcID:   "vpc-1",
		AZs:     []string{"ap-northeast-1a", "ap-northeast-1c"},
	}}

	tests := []struct {
		name      string
		args      []string
		wantCalls []elbOpsCall
		// wantOut は CSV の出力そのもの。tab は空白区切りの語の列で比べるため wantFields を使う。
		wantOut    string
		wantFields []string
	}{
		{
			// 親の elb 単体の一覧。取得関数を ops へ寄せた後も従来どおり動くことを見る。
			name: "elb alone lists load balancers",
			args: []string{"elb", "-p", "dev", "-r", "ap-northeast-1", "-o", "csv"},
			wantCalls: []elbOpsCall{
				{op: "listLoadBalancers", profile: "dev", region: "ap-northeast-1"},
			},
			wantOut: "Name,Type,State,Scheme,DNS,VPC,AZs\n" +
				"lb-1,application,active,internet-facing,lb-1.example.com,vpc-1,\"ap-northeast-1a,ap-northeast-1c\"\n",
		},
		{
			name: "listeners resolves the name and prints csv",
			args: []string{"elb", "listeners", "--lb", "lb-1", "-p", "dev", "-r", "ap-northeast-1", "-o", "csv"},
			wantCalls: []elbOpsCall{
				{op: "listLoadBalancers", profile: "dev", region: "ap-northeast-1"},
				{op: "listListeners", profile: "dev", region: "ap-northeast-1", arg: lbArn},
			},
			wantOut: "Port,Protocol,DefaultAction,TargetGroup,ARN\n" +
				"443,HTTPS,forward,arn:tg-1,arn:listener-1\n",
		},
		{
			name: "rules pass the listener arn and quote the comma in a condition",
			args: []string{"elb", "rules", "--listener", "arn:listener-1", "-p", "dev", "-r", "ap-northeast-1", "-o", "csv"},
			wantCalls: []elbOpsCall{
				{op: "listRules", profile: "dev", region: "ap-northeast-1", arg: "arn:listener-1"},
			},
			wantOut: "Priority,Conditions,Action,TargetGroup,ARN\n" +
				"10,\"host-header=a.example.com,b.example.com path-pattern=/api/*\",forward,arn:tg-1,arn:rule-1\n",
		},
		{
			name: "target-groups pass the lb arn through without listing",
			args: []string{"elb", "target-groups", "--lb", lbArn, "-p", "dev", "-r", "ap-northeast-1", "-o", "csv"},
			wantCalls: []elbOpsCall{
				{op: "listTargetGroups", profile: "dev", region: "ap-northeast-1", arg: lbArn},
			},
			wantOut: "Name,Protocol,Port,TargetType,VPC,HealthCheckPath,ARN\n" +
				"tg-1,HTTP,8080,ip,vpc-1,/health,arn:tg-1\n",
		},
		{
			name: "target-health prints tab",
			args: []string{"elb", "target-health", "--target-group", "arn:tg-1", "-p", "dev", "-r", "ap-northeast-1", "-o", "tab"},
			wantCalls: []elbOpsCall{
				{op: "describeTargetHealth", profile: "dev", region: "ap-northeast-1", arg: "arn:tg-1"},
			},
			// Description に空白を含むため、語の列では 2 語に分かれる。
			wantFields: []string{
				"Target", "Port", "AZ", "State", "Reason", "Description",
				"10.0.1.10", "8080", "ap-northeast-1a", "unhealthy", "Target.FailedHealthChecks", "Health", "checks", "failed",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []elbOpsCall
			root := newRootCmdWithELBOps(t, fakeELBOps(&calls, lbs))
			root.SetArgs(tt.args)

			got := captureStdout(t, root.Execute)

			if diff := cmp.Diff(tt.wantCalls, calls, cmp.AllowUnexported(elbOpsCall{})); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if tt.wantOut != "" && got != tt.wantOut {
				t.Errorf("output = %q, want %q", got, tt.wantOut)
			}
			if tt.wantFields != nil {
				if diff := cmp.Diff(tt.wantFields, strings.Fields(got)); diff != "" {
					t.Errorf("fields mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
