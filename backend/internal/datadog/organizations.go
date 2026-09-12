package datadog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// OrgInfo represents a Datadog organization reachable with the current
// credentials. ID is the lower-cased public_id and Name is the display name.
type OrgInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListOrgs returns the organizations the current credentials can manage
// (GET /api/v1/org). For a parent organization the response contains the
// parent itself and its sub organizations.
//
// public_id は小文字へ正規化してから返す。この値は org としてトークンや
// クライアント登録のファイル名に埋め込まれ (internal/datadogauth の credPath)、
// 受け付ける形は小文字の英数字とハイフンとアンダースコアに限られる
// (datadogauth.ValidateOrg)。ここで正規化しておくことで、API 呼び出しから
// frontend の表示、ファイル名の組み立てまで org を 1 つの小文字の文字列として
// 扱えるようになり、大文字小文字の分岐が生じない。表示名には正規化していない
// Name を使う。
func ListOrgs(ctx context.Context, api *OrganizationsV1API) ([]OrgInfo, error) {
	resp, _, err := api.api.ListOrgs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list datadog orgs: %w", err)
	}

	orgs := []OrgInfo{}
	for _, o := range resp.GetOrgs() {
		id := strings.ToLower(o.GetPublicId())
		if id == "" {
			// public_id を持たない組織は識別子として使えない (空文字は親組織を
			// 指すため、別の組織の認証情報を指してしまう)。落とした事実は残す。
			slog.Warn("datadog returned an organization without a public id; skipping it", "name", o.GetName())
			continue
		}
		orgs = append(orgs, OrgInfo{ID: id, Name: o.GetName()})
	}
	return orgs, nil
}
