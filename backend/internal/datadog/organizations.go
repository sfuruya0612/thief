package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
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

// managedOrgsView は GET /api/v2/org のレスポンスのうち ListOrgs が使う部分だけを、
// 必須フィールドの検証を行わずに読み取るための型。
//
// SDK の ManagedOrgsResponse は、included[] の各組織の attributes に created_at /
// description / disabled / modified_at / name / public_id / sharing / url の 8 つが
// すべて存在し null でないことを要求し、1 つでも欠けるとその組織 (場合によっては
// レスポンス全体) を UnparsedObject (生の map) に退避してゼロ値の構造体にする。
// 実環境のレスポンスはこの要求を満たさないことがあり、その場合 SDK の getter からは
// id も name も取れない (issue 0173)。ListOrgs が必要とするのは組織の id と name と
// current_org / managed_orgs の参照だけなので、SDK の型を一度 JSON に戻してから
// この型で読み直す。SDK の MarshalJSON は UnparsedObject があればその生の値をそのまま
// 書き出すため、SDK が解釈できた部分とできなかった部分のどちらも同じ経路で読める。
type managedOrgsView struct {
	Data struct {
		Relationships struct {
			CurrentOrg struct {
				Data orgRef `json:"data"`
			} `json:"current_org"`
			ManagedOrgs struct {
				Data []orgRef `json:"data"`
			} `json:"managed_orgs"`
		} `json:"relationships"`
	} `json:"data"`
	Included []struct {
		ID         string `json:"id"`
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"included"`
}

// orgRef は JSON:API の relationships が持つ組織への参照。
type orgRef struct {
	ID string `json:"id"`
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

	warnSDKRejections(resp)

	view, err := managedOrgsViewOf(resp)
	if err != nil {
		return nil, fmt.Errorf("decode datadog orgs response: %w", err)
	}

	selfID := strings.ToLower(view.Data.Relationships.CurrentOrg.Data.ID)

	names := make(map[string]string, len(view.Included))
	for _, o := range view.Included {
		id := strings.ToLower(o.ID)
		if id == "" || o.Attributes.Name == "" {
			continue
		}
		names[id] = o.Attributes.Name
	}

	orgs := []OrgInfo{}
	for _, ref := range view.Data.Relationships.ManagedOrgs.Data {
		id := strings.ToLower(ref.ID)
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

// managedOrgsViewOf は SDK のレスポンスを JSON に戻し、必須フィールドの検証を
// 行わない managedOrgsView として読み直す。
func managedOrgsViewOf(resp datadogV2.ManagedOrgsResponse) (managedOrgsView, error) {
	var view managedOrgsView
	raw, err := json.Marshal(resp)
	if err != nil {
		return view, fmt.Errorf("marshal sdk response: %w", err)
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		return view, fmt.Errorf("unmarshal raw response: %w", err)
	}
	return view, nil
}

// warnSDKRejections は SDK が検証に通せず UnparsedObject に退避した部分を警告ログに
// 残す。ListOrgs 自体は生の JSON から読み直すため動作には影響しないが、実環境の
// レスポンスのどのフィールドが SDK の要求と食い違っているかを運用者が把握できる
// ようにする (issue 0173)。
func warnSDKRejections(resp datadogV2.ManagedOrgsResponse) {
	if resp.UnparsedObject == nil {
		for _, o := range resp.Included {
			if o.UnparsedObject != nil {
				warnUnparsedIncludedOrg(o.UnparsedObject)
			}
		}
		return
	}

	// レスポンス全体が退避された場合。included の要素が id / type / attributes
	// のいずれかを欠くと、その要素のデコードエラーがレスポンス全体に波及する。
	slog.Warn("datadog orgs response did not pass the sdk validation; reading it from the raw json")
	items, _ := resp.UnparsedObject["included"].([]interface{})
	for _, item := range items {
		raw, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		var o datadogV2.OrgData
		if err := redecode(raw, &o); err != nil || o.UnparsedObject != nil {
			warnUnparsedIncludedOrg(raw)
		}
	}
}

// warnUnparsedIncludedOrg は SDK が解釈できなかった included の組織 1 件について、
// SDK 自身のデコードエラー文言 (required field description missing など) を添えて
// 警告を出す。SDK の検証規則を自前で再実装せず、SDK が返す理由をそのまま記録する。
func warnUnparsedIncludedOrg(raw map[string]interface{}) {
	id, _ := raw["id"].(string)
	typ, _ := raw["type"].(string)

	reason := "attributes missing"
	if attrs, ok := raw["attributes"]; ok {
		var a datadogV2.OrgAttributes
		switch err := redecode(attrs, &a); {
		case err != nil:
			reason = err.Error()
		case a.UnparsedObject != nil:
			reason = "attributes contain a field of an unexpected type"
		case typ != string(datadogV2.ORGRESOURCETYPE_ORGS):
			reason = "resource type is not orgs"
		default:
			// attributes と type は SDK の要求を満たしている。残る候補は id が UUID
			// として解釈できない場合で、SDK はその理由を返さない。
			reason = "id is not a valid uuid"
		}
	}

	slog.Warn("datadog org in the included resources did not pass the sdk validation; using its raw json instead",
		"id", id, "type", typ, "reason", reason)
}

// redecode は UnparsedObject に退避された生の値を SDK の型で改めてデコードする。
// 戻り値のエラーは SDK の UnmarshalJSON が返したもの。
func redecode(raw interface{}, target interface{}) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal raw value: %w", err)
	}
	return json.Unmarshal(b, target)
}
