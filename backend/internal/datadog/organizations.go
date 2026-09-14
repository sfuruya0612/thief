package datadog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// OrgInfo は現在の認証情報でアクセスできる Datadog の組織を表す。ID は
// 小文字化した UUID、Name は表示名。IsSelf は、この組織が呼び出し元の
// 認証情報自身が属する組織 (API レスポンスの「現在の組織」) かどうかを表す。
// 管理下の他の組織 (Sub Organization) では false になる。
type OrgInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IsSelf bool   `json:"is_self"`
}

// ListOrgs は現在の認証情報が管理できる組織の一覧を返す (GET /api/v2/org)。
// レスポンスは JSON:API 形式で、relationships が呼び出し元自身の組織
// (current_org) と、管理下の全組織 (managed_orgs、呼び出し元自身を含む) を
// 区別している。
//
// GET /api/v1/org (旧実装) は呼び出し元自身の組織 1 件だけを返し、Sub
// Organization を一切含まないことを実機で確認済み (issue 0171)。v1 のレスポンス
// にも自組織かどうかを示すフィールドが無く、Sub Organization の列挙自体ができ
// ないため、v2 のこのエンドポイントに切り替えた。
//
// 識別子は小文字へ正規化してから返す。この値は org としてトークンや
// クライアント登録のファイル名に埋め込まれ (internal/datadogauth の credPath)、
// 受け付ける形は小文字の英数字とハイフンとアンダースコアに限られる
// (datadogauth.ValidateOrg)。v2 API が返す UUID はもともと小文字の 16 進数だが、
// ここでも正規化しておくことで、API 呼び出しから frontend の表示、ファイル名の
// 組み立てまで org を 1 つの小文字の文字列として扱えるようになる。表示名には
// 正規化していない Name を使う。
func ListOrgs(ctx context.Context, api *OrganizationsV2API) ([]OrgInfo, error) {
	resp, _, err := api.api.ListOrgs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list datadog orgs: %w", err)
	}

	selfID := strings.ToLower(resp.Data.Relationships.CurrentOrg.Data.GetId().String())

	names := make(map[string]string, len(resp.Included))
	for _, o := range resp.Included {
		if name := o.Attributes.GetName(); name != "" {
			names[strings.ToLower(o.GetId().String())] = name
		}
	}

	orgs := []OrgInfo{}
	for _, ref := range resp.Data.Relationships.ManagedOrgs.Data {
		id := strings.ToLower(ref.GetId().String())
		name, ok := names[id]
		if !ok {
			// included に対応する名前が無い組織。一覧から消すより id を仮の
			// 表示名にした方が実害が小さいため、警告だけ出して残す。
			slog.Warn("datadog managed org has no matching name in the included resources; using its id as the name", "id", id)
			name = id
		}
		orgs = append(orgs, OrgInfo{ID: id, Name: name, IsSelf: id == selfID})
	}
	return orgs, nil
}
