package tui

import "github.com/sfuruya0612/thief/backend/internal/tidb"

// NewTiDBProvider は TiDB Cloud の Provider を返す。TiDB は AWS のプロファイルや
// Google Cloud のプロジェクトのような一覧の取得範囲を持たず、プロジェクト自体が一覧の
// 対象になる。billedMonth が空なら Cost は当月を表示する。
func NewTiDBProvider(client *tidb.Client, billedMonth string) Provider {
	services := []Service{
		NewTiDBProjectsService(client.ListProjects, client.ListClusters),
		NewTiDBCostService(client.GetCostRange, billedMonth),
	}
	return &provider{
		id:            ProviderTiDB,
		title:         "TiDB Cloud",
		accent:        tidbAccent,
		mutedAccent:   tidbMutedAccent,
		requiresScope: false,
		services:      services,
	}
}

var _ Provider = (*provider)(nil)
