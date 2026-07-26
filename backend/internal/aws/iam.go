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
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	State        string   `json:"state"`
	ARN          string   `json:"arn"`
	Kind         string   `json:"kind"` // user
	MFAEnabled   bool     `json:"mfa_enabled"`
	LastActivity string   `json:"last_activity"`
	Groups       []string `json:"groups"`
	Policies     []string `json:"policies"`
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

// iamFromUser はユーザーの詳細 (MFA / グループ / ポリシー) を取得して IAMResource に変換する。
// 詳細呼び出しの失敗は欠損データとして無視するため、error はキャンセル起因の失敗
// (handleIgnoredErr が伝播させるもの) に限られる。
func iamFromUser(ctx context.Context, client *iam.Client, u iamtypes.User) (IAMResource, error) {
	name := ptrStr(u.UserName)

	// MFA devices (失敗は無視、キャンセル起因は全体エラーとして伝播)
	mfaOut, err := client.ListMFADevices(ctx, &iam.ListMFADevicesInput{UserName: aws.String(name)})
	mfaEnabled := false
	if err == nil {
		mfaEnabled = len(mfaOut.MFADevices) > 0
	} else if err := handleIgnoredErr(err, "list iam mfa devices failed (ignored)", "user", name); err != nil {
		return IAMResource{}, err
	}

	// Groups (失敗は無視、キャンセル起因は全体エラーとして伝播)
	groupsOut, err := client.ListGroupsForUser(ctx, &iam.ListGroupsForUserInput{UserName: aws.String(name)})
	var groups []string
	if err == nil {
		for _, g := range groupsOut.Groups {
			groups = append(groups, ptrStr(g.GroupName))
		}
	} else if err := handleIgnoredErr(err, "list iam groups for user failed (ignored)", "user", name); err != nil {
		return IAMResource{}, err
	}

	// Attached policies (失敗は無視、キャンセル起因は全体エラーとして伝播)
	policiesOut, err := client.ListAttachedUserPolicies(ctx, &iam.ListAttachedUserPoliciesInput{UserName: aws.String(name)})
	var policies []string
	if err == nil {
		for _, p := range policiesOut.AttachedPolicies {
			policies = append(policies, ptrStr(p.PolicyName))
		}
	} else if err := handleIgnoredErr(err, "list iam attached user policies failed (ignored)", "user", name); err != nil {
		return IAMResource{}, err
	}

	var passwordLastUsed *time.Time
	if u.PasswordLastUsed != nil {
		passwordLastUsed = u.PasswordLastUsed
	}

	return newIAMUserResource(ptrStr(u.UserId), name, ptrStr(u.Arn), mfaEnabled, passwordLastUsed, groups, policies), nil
}

func newIAMUserResource(id, name, arn string, mfaEnabled bool, passwordLastUsed *time.Time, groups, policies []string) IAMResource {
	lastActivity := ""
	if passwordLastUsed != nil {
		lastActivity = passwordLastUsed.Format(time.RFC3339)
	}
	return IAMResource{
		ID:           id,
		Name:         name,
		ARN:          arn,
		Kind:         "user",
		MFAEnabled:   mfaEnabled,
		LastActivity: lastActivity,
		Groups:       groups,
		Policies:     policies,
	}
}

// iamFromRole はロールの詳細 (アタッチ済みポリシー) を取得して IAMResource に変換する。
// 詳細呼び出しの失敗は欠損データとして無視するため、error はキャンセル起因の失敗
// (handleIgnoredErr が伝播させるもの) に限られる。
func iamFromRole(ctx context.Context, client *iam.Client, role iamtypes.Role) (IAMResource, error) {
	name := ptrStr(role.RoleName)

	// Attached policies (失敗は無視、キャンセル起因は全体エラーとして伝播)
	policiesOut, err := client.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: aws.String(name)})
	var policies []string
	if err == nil {
		for _, p := range policiesOut.AttachedPolicies {
			policies = append(policies, ptrStr(p.PolicyName))
		}
	} else if err := handleIgnoredErr(err, "list iam attached role policies failed (ignored)", "role", name); err != nil {
		return IAMResource{}, err
	}

	var lastUsed *time.Time
	if role.RoleLastUsed != nil && role.RoleLastUsed.LastUsedDate != nil {
		lastUsed = role.RoleLastUsed.LastUsedDate
	}

	return newIAMRoleResource(ptrStr(role.RoleId), name, ptrStr(role.Arn), lastUsed, policies), nil
}

func newIAMRoleResource(id, name, arn string, lastUsed *time.Time, policies []string) IAMResource {
	lastActivity := ""
	if lastUsed != nil {
		lastActivity = lastUsed.Format(time.RFC3339)
	}
	return IAMResource{
		ID:           id,
		Name:         name,
		ARN:          arn,
		Kind:         "role",
		LastActivity: lastActivity,
		Policies:     policies,
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
