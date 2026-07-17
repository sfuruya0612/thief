// Package snippet はクエリスニペットのファイルベース永続化を提供する。
// スニペットはベースディレクトリ配下のサービス別ディレクトリ (athena / bigquery) に
// <name>.sql として保存されるため、手動で配置した .sql ファイルもそのまま一覧に載る。
package snippet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrInvalidService はサービスキーが未対応の場合のエラー。
var ErrInvalidService = errors.New("invalid snippet service")

// ErrInvalidName は名前がファイル名として使用できない場合のエラー。
var ErrInvalidName = errors.New("invalid snippet name")

// ErrNotFound は指定名のスニペットが存在しない場合のエラー。
var ErrNotFound = errors.New("snippet not found")

// maxNameLength はスニペット名の最大長 (ファイルシステムのファイル名長制限より十分小さい値)。
const maxNameLength = 128

// services は保存を許可するサービスキー (= ベースディレクトリ直下のサブディレクトリ名)。
var services = map[string]bool{
	"athena":   true,
	"bigquery": true,
}

// Snippet は 1 つのクエリスニペット。UpdatedAt はファイルの更新日時。
type Snippet struct {
	Name      string    `json:"name"`
	SQL       string    `json:"sql"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store はサービス別ディレクトリ配下の .sql ファイルとしてスニペットを読み書きする。
type Store struct {
	baseDir string
}

// NewStore は baseDir を保存先ベースディレクトリとする Store を返す。
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func validateService(service string) error {
	if !services[service] {
		return fmt.Errorf("%w: %q", ErrInvalidService, service)
	}
	return nil
}

// validateName はスニペット名がファイル名として安全か検証する。
// パス区切り・NUL・先頭ドット (隠しファイル/相対パス) を拒否する。
func validateName(name string) error {
	if name == "" || len(name) > maxNameLength {
		return fmt.Errorf("%w: must be 1-%d bytes", ErrInvalidName, maxNameLength)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("%w: must not start with a dot", ErrInvalidName)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("%w: must not contain path separators", ErrInvalidName)
	}
	return nil
}

func (s *Store) dir(service string) string {
	return filepath.Join(s.baseDir, service)
}

func (s *Store) path(service, name string) string {
	return filepath.Join(s.baseDir, service, name+".sql")
}

// List は service のディレクトリ直下の .sql ファイルを更新日時の降順
// (同時刻は名前順) で返す。ディレクトリが存在しない場合は空リストを返す (初回起動時)。
func (s *Store) List(service string) ([]Snippet, error) {
	if err := validateService(service); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir(service))
	if errors.Is(err, os.ErrNotExist) {
		return []Snippet{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snippets dir %s: %w", s.dir(service), err)
	}
	snippets := make([]Snippet, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".sql")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") || validateName(name) != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir(service), e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read snippet %s: %w", e.Name(), err)
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat snippet %s: %w", e.Name(), err)
		}
		snippets = append(snippets, Snippet{
			Name:      name,
			SQL:       string(data),
			UpdatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(snippets, func(i, j int) bool {
		if !snippets[i].UpdatedAt.Equal(snippets[j].UpdatedAt) {
			return snippets[i].UpdatedAt.After(snippets[j].UpdatedAt)
		}
		return snippets[i].Name < snippets[j].Name
	})
	return snippets, nil
}

// Save は service 配下に name のスニペットを作成または上書きし、保存結果を返す。
// 一時ファイルへ書き込んでから rename することで部分書き込みを防ぐ。
func (s *Store) Save(service, name, sql string) (Snippet, error) {
	if err := validateService(service); err != nil {
		return Snippet{}, err
	}
	if err := validateName(name); err != nil {
		return Snippet{}, err
	}
	dir := s.dir(service)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Snippet{}, fmt.Errorf("create snippets dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return Snippet{}, fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name()) // rename 成功後は ENOENT になるだけなので常に呼んでよい
	if _, err := tmp.WriteString(sql); err != nil {
		tmp.Close()
		return Snippet{}, fmt.Errorf("write snippet %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return Snippet{}, fmt.Errorf("close snippet %s: %w", name, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return Snippet{}, fmt.Errorf("chmod snippet %s: %w", name, err)
	}
	p := s.path(service, name)
	if err := os.Rename(tmp.Name(), p); err != nil {
		return Snippet{}, fmt.Errorf("rename snippet %s: %w", name, err)
	}
	info, err := os.Stat(p)
	if err != nil {
		return Snippet{}, fmt.Errorf("stat snippet %s: %w", name, err)
	}
	return Snippet{Name: name, SQL: sql, UpdatedAt: info.ModTime().UTC()}, nil
}

// Delete は service 配下の name のスニペットを削除する。存在しない場合は ErrNotFound を返す。
func (s *Store) Delete(service, name string) error {
	if err := validateService(service); err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	err := os.Remove(s.path(service, name))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("delete snippet %s: %w", name, err)
	}
	return nil
}
