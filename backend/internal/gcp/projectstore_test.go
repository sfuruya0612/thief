package gcp

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSaveAndLoadProjectsFromDisk(t *testing.T) {
	dir := t.TempDir()

	want := []ProjectInfo{
		{ProjectID: "proj-1", Name: "Project One", ProjectNumber: 123, State: "ACTIVE"},
		{ProjectID: "proj-2", Name: "Project Two", ProjectNumber: 456, State: "ACTIVE"},
	}
	fetchedAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	if err := SaveProjectsToDisk(dir, want, fetchedAt); err != nil {
		t.Fatalf("SaveProjectsToDisk() error = %v", err)
	}

	got, gotFetchedAt, ok, err := LoadProjectsFromDisk(dir)
	if err != nil {
		t.Fatalf("LoadProjectsFromDisk() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadProjectsFromDisk() ok = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadProjectsFromDisk() projects = %+v, want %+v", got, want)
	}
	if !gotFetchedAt.Equal(fetchedAt) {
		t.Errorf("LoadProjectsFromDisk() fetchedAt = %v, want %v", gotFetchedAt, fetchedAt)
	}
}

func TestLoadProjectsFromDisk_NotExist(t *testing.T) {
	dir := t.TempDir()

	projects, _, ok, err := LoadProjectsFromDisk(dir)
	if err != nil {
		t.Fatalf("LoadProjectsFromDisk() error = %v", err)
	}
	if ok {
		t.Error("LoadProjectsFromDisk() ok = true, want false")
	}
	if projects != nil {
		t.Errorf("LoadProjectsFromDisk() projects = %+v, want nil", projects)
	}
}

func TestSaveProjectsToDisk_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "thief")

	if err := SaveProjectsToDisk(dir, []ProjectInfo{{ProjectID: "proj-1"}}, time.Now()); err != nil {
		t.Fatalf("SaveProjectsToDisk() error = %v", err)
	}

	_, _, ok, err := LoadProjectsFromDisk(dir)
	if err != nil {
		t.Fatalf("LoadProjectsFromDisk() error = %v", err)
	}
	if !ok {
		t.Error("LoadProjectsFromDisk() ok = false, want true after save into nested dir")
	}
}
