package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"golang.org/x/sync/errgroup"
)

// IAMResource represents an IAM user.
type IAMResource struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	State                 string   `json:"state"`
	ARN                   string   `json:"arn"`
	Kind                  string   `json:"kind"` // user
	MFAEnabled            bool     `json:"mfa_enabled"`
	MFAEnabledFetchFailed bool     `json:"mfa_enabled_fetch_failed,omitempty"`
	LastActivity          string   `json:"last_activity"`
	Groups                []string `json:"groups"`
	GroupsFetchFailed     bool     `json:"groups_fetch_failed,omitempty"`
	Policies              []string `json:"policies"`
	PoliciesFetchFailed   bool     `json:"policies_fetch_failed,omitempty"`
}

func (r IAMResource) ResourceID() string    { return r.ID }
func (r IAMResource) ResourceName() string  { return r.Name }
func (r IAMResource) ResourceState() string { return "active" }
func (r IAMResource) ServiceName() string   { return "iam" }

// iamDetailConcurrency はユーザー/ロールごとの詳細取得 (MFA / グループ / ポリシー) を同時実行
// する上限数。IAM のマネジメント系 API は他サービスよりレート制限が厳しいため控えめに始める
// (issue 0081 の設計判断)。
const iamDetailConcurrency = 10

// ListIAMResources returns all IAM users and roles for the given profile.
// IAM is a global service; region is ignored.
func ListIAMResources(ctx context.Context, profile, _ string) ([]IAMResource, error) {
	// 各フェーズの所要時間を計測してログに残す (issue 0081: クライアント生成の区間は
	// issue 0083 の GetSession キャッシュ化調査の入力を兼ねる)。
	overallStart := time.Now()

	clientStart := time.Now()
	client, err := newIAMClient(ctx, profile)
	if err != nil {
		return nil, err
	}
	slog.Info("iam client created",
		"profile", profile, "duration_ms", time.Since(clientStart).Milliseconds())

	// 詳細取得を並列化するため、先にユーザーとロールの全件をページングで収集する
	// (ページングはトークンが前ページに依存するため直列のまま)。
	listStart := time.Now()
	var users []iamtypes.User
	userPaginator := iam.NewListUsersPaginator(client, &iam.ListUsersInput{})
	for userPaginator.HasMorePages() {
		page, err := userPaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list iam users: %w", err)
		}
		users = append(users, page.Users...)
	}

	var roles []iamtypes.Role
	rolePaginator := iam.NewListRolesPaginator(client, &iam.ListRolesInput{})
	for rolePaginator.HasMorePages() {
		page, err := rolePaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list iam roles: %w", err)
		}
		roles = append(roles, page.Roles...)
	}
	slog.Info("iam list users and roles done",
		"profile", profile, "duration_ms", time.Since(listStart).Milliseconds(),
		"users", len(users), "roles", len(roles))

	// ユーザー/ロールごとの詳細取得は互いに独立しているため並列実行する。各 goroutine は
	// 自分の index にのみ書き込むため結果スライスへの書き込みはロック不要で競合しない
	// (データオーナーシップを goroutine ごとに分離、ListS3Resources と同型)。
	// 返却順は並列化前と同じユーザー (一覧順) → ロール (一覧順) に保つ。
	detailStart := time.Now()
	resources := make([]IAMResource, len(users)+len(roles))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(iamDetailConcurrency)
	for i, u := range users {
		g.Go(func() error {
			r, err := iamFromUser(gctx, client, u)
			if err != nil {
				return err
			}
			resources[i] = r
			return nil
		})
	}
	for i, role := range roles {
		g.Go(func() error {
			r, err := iamFromRole(gctx, client, role)
			if err != nil {
				return err
			}
			resources[len(users)+i] = r
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slog.Info("iam get details done",
		"profile", profile, "duration_ms", time.Since(detailStart).Milliseconds(),
		"concurrency", iamDetailConcurrency, "count", len(resources))

	slog.Info("iam list all done",
		"profile", profile, "duration_ms", time.Since(overallStart).Milliseconds(), "count", len(resources))
	return resources, nil
}

// iamUserDetailClient は iamFromUser が要求する API 呼び出しを抽象化する。
type iamUserDetailClient interface {
	ListMFADevices(ctx context.Context, params *iam.ListMFADevicesInput, optFns ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error)
	ListGroupsForUser(ctx context.Context, params *iam.ListGroupsForUserInput, optFns ...func(*iam.Options)) (*iam.ListGroupsForUserOutput, error)
	ListAttachedUserPolicies(ctx context.Context, params *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error)
}

// iamRoleDetailClient は iamFromRole が要求する API 呼び出しを抽象化する。
type iamRoleDetailClient interface {
	ListAttachedRolePolicies(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
}

// iamFromUser はユーザーの詳細 (MFA / グループ / ポリシー) を取得して IAMResource に変換する。
// 詳細呼び出しの失敗は欠損データとして無視するため、error はキャンセル起因の失敗
// (handleIgnoredErr が伝播させるもの) に限られる。
func iamFromUser(ctx context.Context, client iamUserDetailClient, u iamtypes.User) (IAMResource, error) {
	name := ptrStr(u.UserName)

	// MFA devices (失敗は無視、キャンセル起因は全体エラーとして伝播)
	mfaOut, mfaErr := client.ListMFADevices(ctx, &iam.ListMFADevicesInput{UserName: aws.String(name)})
	mfaEnabled := false
	var mfaEnabledFetchFailed bool
	if mfaErr == nil {
		mfaEnabled = len(mfaOut.MFADevices) > 0
	} else {
		degraded, err := handleIgnoredErrFlag(mfaErr, "list iam mfa devices failed (ignored)", "user", name)
		if err != nil {
			return IAMResource{}, err
		}
		mfaEnabledFetchFailed = degraded
	}

	// Groups (失敗は無視、キャンセル起因は全体エラーとして伝播)
	groupsOut, groupsErr := client.ListGroupsForUser(ctx, &iam.ListGroupsForUserInput{UserName: aws.String(name)})
	var groups []string
	var groupsFetchFailed bool
	if groupsErr == nil {
		for _, g := range groupsOut.Groups {
			groups = append(groups, ptrStr(g.GroupName))
		}
	} else {
		degraded, err := handleIgnoredErrFlag(groupsErr, "list iam groups for user failed (ignored)", "user", name)
		if err != nil {
			return IAMResource{}, err
		}
		groupsFetchFailed = degraded
	}

	// Attached policies (失敗は無視、キャンセル起因は全体エラーとして伝播)
	policiesOut, policiesErr := client.ListAttachedUserPolicies(ctx, &iam.ListAttachedUserPoliciesInput{UserName: aws.String(name)})
	var policies []string
	var policiesFetchFailed bool
	if policiesErr == nil {
		for _, p := range policiesOut.AttachedPolicies {
			policies = append(policies, ptrStr(p.PolicyName))
		}
	} else {
		degraded, err := handleIgnoredErrFlag(policiesErr, "list iam attached user policies failed (ignored)", "user", name)
		if err != nil {
			return IAMResource{}, err
		}
		policiesFetchFailed = degraded
	}

	var passwordLastUsed *time.Time
	if u.PasswordLastUsed != nil {
		passwordLastUsed = u.PasswordLastUsed
	}

	return newIAMUserResource(ptrStr(u.UserId), name, ptrStr(u.Arn), mfaEnabled, mfaEnabledFetchFailed, passwordLastUsed, groups, groupsFetchFailed, policies, policiesFetchFailed), nil
}

func newIAMUserResource(id, name, arn string, mfaEnabled, mfaEnabledFetchFailed bool, passwordLastUsed *time.Time, groups []string, groupsFetchFailed bool, policies []string, policiesFetchFailed bool) IAMResource {
	lastActivity := ""
	if passwordLastUsed != nil {
		lastActivity = passwordLastUsed.Format(time.RFC3339)
	}
	return IAMResource{
		ID:                    id,
		Name:                  name,
		ARN:                   arn,
		Kind:                  "user",
		MFAEnabled:            mfaEnabled,
		MFAEnabledFetchFailed: mfaEnabledFetchFailed,
		LastActivity:          lastActivity,
		Groups:                groups,
		GroupsFetchFailed:     groupsFetchFailed,
		Policies:              policies,
		PoliciesFetchFailed:   policiesFetchFailed,
	}
}

// iamFromRole はロールの詳細 (アタッチ済みポリシー) を取得して IAMResource に変換する。
// 詳細呼び出しの失敗は欠損データとして無視するため、error はキャンセル起因の失敗
// (handleIgnoredErr が伝播させるもの) に限られる。
func iamFromRole(ctx context.Context, client iamRoleDetailClient, role iamtypes.Role) (IAMResource, error) {
	name := ptrStr(role.RoleName)

	// Attached policies (失敗は無視、キャンセル起因は全体エラーとして伝播)
	policiesOut, policiesErr := client.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: aws.String(name)})
	var policies []string
	var policiesFetchFailed bool
	if policiesErr == nil {
		for _, p := range policiesOut.AttachedPolicies {
			policies = append(policies, ptrStr(p.PolicyName))
		}
	} else {
		degraded, err := handleIgnoredErrFlag(policiesErr, "list iam attached role policies failed (ignored)", "role", name)
		if err != nil {
			return IAMResource{}, err
		}
		policiesFetchFailed = degraded
	}

	var lastUsed *time.Time
	if role.RoleLastUsed != nil && role.RoleLastUsed.LastUsedDate != nil {
		lastUsed = role.RoleLastUsed.LastUsedDate
	}

	return newIAMRoleResource(ptrStr(role.RoleId), name, ptrStr(role.Arn), lastUsed, policies, policiesFetchFailed), nil
}

func newIAMRoleResource(id, name, arn string, lastUsed *time.Time, policies []string, policiesFetchFailed bool) IAMResource {
	lastActivity := ""
	if lastUsed != nil {
		lastActivity = lastUsed.Format(time.RFC3339)
	}
	return IAMResource{
		ID:                    id,
		Name:                  name,
		ARN:                   arn,
		Kind:                  "role",
		MFAEnabledFetchFailed: false,
		LastActivity:          lastActivity,
		Groups:                nil,
		GroupsFetchFailed:     false,
		Policies:              policies,
		PoliciesFetchFailed:   policiesFetchFailed,
	}
}

// IAMUserInfo はレガシー CLI 互換の IAM ユーザー表示用フィールドを保持する。
type IAMUserInfo struct {
	UserName   string
	UserID     string
	Groups     string // カンマ区切りのグループ名
	Policies   string // カンマ区切りのアタッチ済みマネージドポリシー名
	CreateDate string
}

// ToRow converts IAMUserInfo to a string slice suitable for table formatting.
func (u IAMUserInfo) ToRow() []string {
	return []string{u.UserName, u.UserID, u.Groups, u.Policies, u.CreateDate}
}

// ListIAMUserInfos は全 IAM ユーザーを所属グループ・アタッチ済みポリシーとともに返す。
// ListIAMResources と異なりユーザーのみを対象とし、取得失敗はエラーとして伝播する。
func ListIAMUserInfos(ctx context.Context, profile string) ([]IAMUserInfo, error) {
	client, err := newIAMClient(ctx, profile)
	if err != nil {
		return nil, err
	}

	var users []IAMUserInfo
	paginator := iam.NewListUsersPaginator(client, &iam.ListUsersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list iam users: %w", err)
		}
		for _, u := range page.Users {
			userName := ptrStr(u.UserName)

			groups, err := listIAMGroupNamesForUser(ctx, client, userName)
			if err != nil {
				return nil, fmt.Errorf("list groups for user %s: %w", userName, err)
			}

			policies, err := listIAMAttachedPolicyNamesForUser(ctx, client, userName)
			if err != nil {
				return nil, fmt.Errorf("list policies for user %s: %w", userName, err)
			}

			createDate := ""
			if u.CreateDate != nil {
				createDate = u.CreateDate.String()
			}

			users = append(users, IAMUserInfo{
				UserName:   userName,
				UserID:     ptrStr(u.UserId),
				Groups:     strings.Join(groups, ","),
				Policies:   strings.Join(policies, ","),
				CreateDate: createDate,
			})
		}
	}
	return users, nil
}

// listIAMGroupNamesForUser はユーザーが所属する全グループ名を返す。
func listIAMGroupNamesForUser(ctx context.Context, client *iam.Client, userName string) ([]string, error) {
	var names []string
	paginator := iam.NewListGroupsForUserPaginator(client, &iam.ListGroupsForUserInput{
		UserName: aws.String(userName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, g := range page.Groups {
			names = append(names, ptrStr(g.GroupName))
		}
	}
	return names, nil
}

// listIAMAttachedPolicyNamesForUser はユーザーに直接アタッチされた全マネージドポリシー名を返す。
func listIAMAttachedPolicyNamesForUser(ctx context.Context, client *iam.Client, userName string) ([]string, error) {
	var names []string
	paginator := iam.NewListAttachedUserPoliciesPaginator(client, &iam.ListAttachedUserPoliciesInput{
		UserName: aws.String(userName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range page.AttachedPolicies {
			names = append(names, ptrStr(p.PolicyName))
		}
	}
	return names, nil
}

// newIAMClient は IAM API クライアントを生成する。IAM はグローバルサービスのため us-east-1 を使う。
func newIAMClient(ctx context.Context, profile string) (*iam.Client, error) {
	return NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *iam.Client {
		return iam.NewFromConfig(cfg)
	})
}
