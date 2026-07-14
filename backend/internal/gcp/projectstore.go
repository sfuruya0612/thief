package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// projectStoreFileName はプロジェクト一覧のローカルキャッシュファイル名。
// 保存先ディレクトリは呼び出し側 (Server) が config.Dir() で解決して渡す。
const projectStoreFileName = "gcp-projects.json"

// projectStoreFile はローカルキャッシュファイルの JSON 形式。
type projectStoreFile struct {
	FetchedAt time.Time     `json:"fetched_at"`
	Projects  []ProjectInfo `json:"projects"`
}

// LoadProjectsFromDisk は dir 配下のローカルキャッシュファイルからプロジェクト一覧を読む。
// ファイルが存在しない場合は空スライスと ok=false を返す (エラーではない)。
func LoadProjectsFromDisk(dir string) ([]ProjectInfo, time.Time, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, projectStoreFileName))
	if os.IsNotExist(err) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("read gcp projects cache: %w", err)
	}
	var f projectStoreFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, time.Time{}, false, fmt.Errorf("parse gcp projects cache: %w", err)
	}
	return f.Projects, f.FetchedAt, true, nil
}

// SaveProjectsToDisk は projects を dir 配下のローカルキャッシュファイルに書き込む。
// ディレクトリが存在しない場合は作成する。
func SaveProjectsToDisk(dir string, projects []ProjectInfo, fetchedAt time.Time) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create gcp projects cache dir: %w", err)
	}
	data, err := json.MarshalIndent(projectStoreFile{FetchedAt: fetchedAt, Projects: projects}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal gcp projects cache: %w", err)
	}
	path := filepath.Join(dir, projectStoreFileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write gcp projects cache: %w", err)
	}
	return nil
}

// RefreshProjectsOnDisk は Cloud Resource Manager から最新のプロジェクト一覧を取得し、
// dir 配下のローカルキャッシュファイルを上書きする。取得できた一覧を返す。
func RefreshProjectsOnDisk(ctx context.Context, dir string) ([]ProjectInfo, error) {
	projects, err := ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if err := SaveProjectsToDisk(dir, projects, time.Now()); err != nil {
		return nil, err
	}
	return projects, nil
}
