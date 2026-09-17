package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenDir はゴールデンファイルの置き場所。frontend の型検査が同一ファイルを import する。
const goldenDir = "../../../frontend/src/types/__contract__"

// marshalGolden は e の型のインスタンスをフィラーで生成し、ゴールデンの表現 (2 スペース
// インデントの JSON + 末尾改行) にエンコードする。
func marshalGolden(t *testing.T, e Entry) []byte {
	t.Helper()
	filled, err := Fill(e.Value)
	if err != nil {
		t.Fatalf("fill %s: %v", e.Name, err)
	}
	data, err := json.MarshalIndent(filled, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", e.Name, err)
	}
	return append(data, '\n')
}

// TestGolden は契約対象の各型の生成結果がコミット済みのゴールデンと一致することを検証する。
// UPDATE_GOLDEN=1 を付けて実行すると、比較の代わりにゴールデンを再生成する。
func TestGolden(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	if update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
	}
	for _, e := range Registry {
		t.Run(e.Name, func(t *testing.T) {
			got := marshalGolden(t, e)
			path := filepath.Join(goldenDir, e.Name+".json")
			if update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v (run 'UPDATE_GOLDEN=1 go test ./internal/contract/' to generate)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("golden mismatch for %s: struct JSON tags changed; regenerate with 'UPDATE_GOLDEN=1 go test ./internal/contract/' and review frontend type check results\n--- got ---\n%s\n--- want ---\n%s", e.Name, got, want)
			}
		})
	}
}

// tagsGoldenPath は json タグの一覧のゴールデンの置き場所。frontend は参照しないため
// backend の慣例に従い testdata に置く。
const tagsGoldenPath = "testdata/tags.golden"

// marshalTags はレジストリ全型のフィールド名と json タグの一覧を、型名の見出し付きの
// テキストにエンコードする。
func marshalTags() []byte {
	var buf bytes.Buffer
	for _, e := range Registry {
		fmt.Fprintf(&buf, "== %s\n", e.Name)
		for _, line := range Tags(e.Value) {
			fmt.Fprintln(&buf, line)
		}
	}
	return buf.Bytes()
}

// TestTagsGolden は json タグの一覧がコミット済みのゴールデンと一致することを検証する。
// フィラーが全フィールドに非ゼロ値を入れるため、omitempty の増減はゴールデン JSON の
// 比較 (TestGolden) に現れない。このテストがタグの変更そのものを検出する。
// UPDATE_GOLDEN=1 を付けて実行すると、比較の代わりにゴールデンを再生成する。
func TestTagsGolden(t *testing.T) {
	got := marshalTags()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(tagsGoldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(tagsGoldenPath, got, 0o644); err != nil {
			t.Fatalf("write tags golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(tagsGoldenPath)
	if err != nil {
		t.Fatalf("read tags golden: %v (run 'UPDATE_GOLDEN=1 go test ./internal/contract/' to generate)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("tags golden mismatch: struct JSON tags changed; regenerate with 'UPDATE_GOLDEN=1 go test ./internal/contract/' and review frontend type check results\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFillDeterministic はフィラーの生成が決定的であること (同一コードで 2 回生成した
// 結果が一致すること) を検証する。
func TestFillDeterministic(t *testing.T) {
	for _, e := range Registry {
		t.Run(e.Name, func(t *testing.T) {
			first := marshalGolden(t, e)
			second := marshalGolden(t, e)
			if !bytes.Equal(first, second) {
				t.Errorf("fill is not deterministic for %s:\n--- first ---\n%s\n--- second ---\n%s", e.Name, first, second)
			}
		})
	}
}

// TestFillUnsupportedKind はフィラーが対応しない型のフィールドで、フィールド経路を含む
// エラーを返すことを検証する。レジストリの現行 64 型はこの分岐に到達しないため、将来
// 対応しない型のフィールドが追加された場合の挙動をローカル定義の型で保証する。
func TestFillUnsupportedKind(t *testing.T) {
	type chanField struct {
		C chan int `json:"c"`
	}
	type funcField struct {
		F func() `json:"f"`
	}
	type anyField struct {
		V any `json:"v"`
	}
	type sliceOfChan struct {
		S []chan int `json:"s"`
	}
	tests := []struct {
		name     string
		value    any
		wantPath string
	}{
		{name: "chan", value: chanField{}, wantPath: "chanField.C"},
		{name: "func", value: funcField{}, wantPath: "funcField.F"},
		{name: "interface", value: anyField{}, wantPath: "anyField.V"},
		{name: "slice element", value: sliceOfChan{}, wantPath: "sliceOfChan.S[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Fill(tt.value)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported kind") {
				t.Errorf("error %q does not mention unsupported kind", err.Error())
			}
			if !strings.Contains(err.Error(), tt.wantPath) {
				t.Errorf("error %q does not contain path %q", err.Error(), tt.wantPath)
			}
		})
	}
}

// TestRegistryNamesUnique はレジストリの型名 (ゴールデンファイル名) の重複を検出する。
func TestRegistryNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Registry {
		if seen[e.Name] {
			t.Errorf("duplicate registry name: %s", e.Name)
		}
		seen[e.Name] = true
	}
	if len(Registry) != 64 {
		t.Errorf("registry has %d entries, want 64", len(Registry))
	}
}

// TestGoldenDirHasNoStrayFiles はゴールデン置き場にレジストリへ対応しない JSON が
// 残っていないことを検証する (型の削除や改名でゴールデンが取り残されるのを防ぐ)。
func TestGoldenDirHasNoStrayFiles(t *testing.T) {
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("read golden dir: %v", err)
	}
	known := map[string]bool{}
	for _, e := range Registry {
		known[e.Name+".json"] = true
	}
	for _, f := range entries {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		if !known[f.Name()] {
			t.Errorf("stray golden file without registry entry: %s", f.Name())
		}
	}
}

// ExampleFill はフィラーの決定的な生成値の形を示す。
func ExampleFill() {
	type sample struct {
		Name  string            `json:"name"`
		Count int               `json:"count"`
		Tags  map[string]string `json:"tags"`
	}
	filled, err := Fill(sample{})
	if err != nil {
		fmt.Println(err)
		return
	}
	data, err := json.Marshal(filled)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(data))
	// Output: {"name":"s1","count":2,"tags":{"s3":"s4"}}
}
