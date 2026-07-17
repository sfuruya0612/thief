package snippet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateService(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "athena", input: "athena"},
		{name: "bigquery", input: "bigquery"},
		{name: "empty", input: "", wantErr: ErrInvalidService},
		{name: "unknown", input: "redshift", wantErr: ErrInvalidService},
		{name: "traversal", input: "../athena", wantErr: ErrInvalidService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateService(tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateService(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "ok", input: "monthly cost"},
		{name: "ok japanese", input: "月次コスト集計"},
		{name: "empty", input: "", wantErr: ErrInvalidName},
		{name: "too long", input: string(make([]byte, maxNameLength+1)), wantErr: ErrInvalidName},
		{name: "leading dot", input: ".hidden", wantErr: ErrInvalidName},
		{name: "slash", input: "a/b", wantErr: ErrInvalidName},
		{name: "backslash", input: `a\b`, wantErr: ErrInvalidName},
		{name: "traversal", input: "../evil", wantErr: ErrInvalidName},
		{name: "nul", input: "a\x00b", wantErr: ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateName(tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateName(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestStoreSaveListRoundTrip(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "snippets"))

	if _, err := s.Save("athena", "first", "SELECT 1"); err != nil {
		t.Fatalf("Save(first): %v", err)
	}
	if _, err := s.Save("athena", "second", "SELECT 2"); err != nil {
		t.Fatalf("Save(second): %v", err)
	}

	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List returned %d snippets, want 2", len(got))
	}
	byName := map[string]Snippet{}
	for _, sn := range got {
		byName[sn.Name] = sn
	}
	if byName["first"].SQL != "SELECT 1" || byName["second"].SQL != "SELECT 2" {
		t.Errorf("List content mismatch: %+v", got)
	}
	for _, sn := range got {
		if sn.UpdatedAt.IsZero() {
			t.Errorf("UpdatedAt of %s is zero", sn.Name)
		}
	}
}

func TestStoreSeparatesServices(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Save("athena", "a", "SELECT 1"); err != nil {
		t.Fatalf("Save(athena): %v", err)
	}
	if _, err := s.Save("bigquery", "b", "SELECT 2"); err != nil {
		t.Fatalf("Save(bigquery): %v", err)
	}

	// サービス別のサブディレクトリに保存される
	if _, err := os.Stat(filepath.Join(dir, "athena", "a.sql")); err != nil {
		t.Errorf("athena/a.sql: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bigquery", "b.sql")); err != nil {
		t.Errorf("bigquery/b.sql: %v", err)
	}

	// 一覧は互いに混ざらない
	athena, err := s.List("athena")
	if err != nil {
		t.Fatalf("List(athena): %v", err)
	}
	if len(athena) != 1 || athena[0].Name != "a" {
		t.Errorf("List(athena) = %+v, want only a", athena)
	}
	bigquery, err := s.List("bigquery")
	if err != nil {
		t.Fatalf("List(bigquery): %v", err)
	}
	if len(bigquery) != 1 || bigquery[0].Name != "b" {
		t.Errorf("List(bigquery) = %+v, want only b", bigquery)
	}
}

func TestStoreRejectsInvalidService(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.List("redshift"); !errors.Is(err, ErrInvalidService) {
		t.Errorf("List = %v, want ErrInvalidService", err)
	}
	if _, err := s.Save("redshift", "q", "SELECT 1"); !errors.Is(err, ErrInvalidService) {
		t.Errorf("Save = %v, want ErrInvalidService", err)
	}
	if err := s.Delete("redshift", "q"); !errors.Is(err, ErrInvalidService) {
		t.Errorf("Delete = %v, want ErrInvalidService", err)
	}
}

func TestStoreSaveOverwrites(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Save("athena", "q", "SELECT 1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.Save("athena", "q", "SELECT 2"); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].SQL != "SELECT 2" {
		t.Errorf("List = %+v, want single snippet with SELECT 2", got)
	}
}

func TestStoreSaveRejectsInvalidName(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Save("athena", "../evil", "SELECT 1"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save(../evil) = %v, want ErrInvalidName", err)
	}
}

func TestStoreListSortsByModTimeDesc(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Save("athena", "old", "SELECT 1"); err != nil {
		t.Fatalf("Save(old): %v", err)
	}
	if _, err := s.Save("athena", "new", "SELECT 2"); err != nil {
		t.Fatalf("Save(new): %v", err)
	}
	// mtime を明示的にずらしてソート順を検証する
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "athena", "old.sql"), past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].Name != "new" || got[1].Name != "old" {
		t.Errorf("List order = %+v, want [new old]", got)
	}
}

func TestStoreListMissingDirReturnsEmpty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "does-not-exist"))
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List = %+v, want empty", got)
	}
}

func TestStoreListSkipsNonSnippetEntries(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	s := NewStore(base)
	// 手動配置の .sql は一覧に載る
	if err := os.WriteFile(filepath.Join(dir, "manual.sql"), []byte("SELECT 3"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// .sql 以外・隠しファイル・ディレクトリは無視される
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp-123"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.sql"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "manual" || got[0].SQL != "SELECT 3" {
		t.Errorf("List = %+v, want only manual", got)
	}
}

func TestStoreDelete(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Save("athena", "q", "SELECT 1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete("athena", "q"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List after delete = %+v, want empty", got)
	}
}

func TestStoreDeleteMissingReturnsNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Delete("athena", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(nope) = %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteRejectsInvalidName(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Delete("athena", "../evil"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Delete(../evil) = %v, want ErrInvalidName", err)
	}
}
