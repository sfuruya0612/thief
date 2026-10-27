package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

func TestGCSBucketService_ColumnsAndRow(t *testing.T) {
	svc := NewGCSBucketService(func(_ context.Context, projectID string) ([]gcp.BucketInfo, error) {
		if projectID != "proj" {
			t.Errorf("projectID = %q, want proj", projectID)
		}
		return []gcp.BucketInfo{{
			Name:         "assets",
			Location:     "ASIA-NORTHEAST1",
			StorageClass: "STANDARD",
			CreateTime:   "2026-10-01T00:00:00Z",
			UpdateTime:   "2026-10-02T00:00:00Z",
		}}, nil
	})

	if svc.ID() != "gcs" || svc.Title() != "Cloud Storage" || svc.SupportsSession() {
		t.Errorf("service = %s/%s session=%v", svc.ID(), svc.Title(), svc.SupportsSession())
	}
	if got, want := serviceTabTitles(svc), []string{"Overview"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tabs = %v, want %v", got, want)
	}
	wantColumns := []Column{{Title: "Bucket"}, {Title: "Location"}, {Title: "Storage class"}, {Title: "Created"}}
	if !reflect.DeepEqual(svc.Columns(), wantColumns) {
		t.Errorf("columns = %#v, want %#v", svc.Columns(), wantColumns)
	}

	rows, err := svc.Load(context.Background(), "proj", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := rows[0].(gcsBucketRow)
	if row.ID() != "assets" || row.Name() != "assets" || row.State() != "" {
		t.Errorf("row = %q/%q/%q", row.ID(), row.Name(), row.State())
	}
	wantCells := []string{"assets", "ASIA-NORTHEAST1", "STANDARD", "2026-10-01T00:00:00Z"}
	if !reflect.DeepEqual(row.Cells(), wantCells) {
		t.Errorf("cells = %v, want %v", row.Cells(), wantCells)
	}
	wantOverview := []KV{
		{Key: "Bucket", Value: "assets"},
		{Key: "Location", Value: "ASIA-NORTHEAST1"},
		{Key: "Storage class", Value: "STANDARD"},
		{Key: "Created", Value: "2026-10-01T00:00:00Z"},
	}
	if !reflect.DeepEqual(row.Overview(), wantOverview) {
		t.Errorf("overview = %#v, want %#v", row.Overview(), wantOverview)
	}
}

func TestGCSBucketService_LoadError(t *testing.T) {
	svc := NewGCSBucketService(func(context.Context, string) ([]gcp.BucketInfo, error) {
		return nil, errors.New("permission denied")
	})
	_, err := svc.Load(context.Background(), "proj", "")
	if err == nil || !strings.Contains(err.Error(), "list Cloud Storage buckets: permission denied") {
		t.Errorf("err = %v", err)
	}
}

func TestGCSObjectsTab_UsesTheSharedObjectBrowser(t *testing.T) {
	svc := NewGCSBucketService(
		func(context.Context, string) ([]gcp.BucketInfo, error) { return nil, nil },
		func(_ context.Context, projectID, bucket, prefix, delimiter string) ([]gcp.ObjectInfo, []string, bool, error) {
			if projectID != "proj" || bucket != "assets" {
				t.Errorf("list args = %q/%q, want proj/assets", projectID, bucket)
			}
			if prefix != "" || delimiter != "/" {
				t.Errorf("list prefix/delimiter = %q/%q, want an empty hierarchy root", prefix, delimiter)
			}
			return []gcp.ObjectInfo{{
				Name: "notes/sample.txt", Bucket: bucket, Size: 12,
				StorageClass: "STANDARD", Updated: "2026-10-02T00:00:00Z",
			}}, []string{"reports/"}, false, nil
		},
	)
	specs := svc.(TabProvider).Tabs()
	if len(specs) != 2 || specs[1].Title != "Objects" {
		t.Fatalf("tabs = %#v", specs)
	}
	tab := specs[1].New(context.Background(), "proj", "", gcsBucketRow{res: gcp.BucketInfo{Name: "assets"}})
	tab = loadTab(t, tab)
	objects, ok := tab.(*s3ObjectsTab)
	if !ok {
		t.Fatalf("tab = %T, want the shared object browser", tab)
	}
	if objects.keyTitle != "Object" || objects.updatedTitle != "Updated" {
		t.Errorf("column titles = %q/%q, want Object/Updated", objects.keyTitle, objects.updatedTitle)
	}
	view := tab.View(80, 20)
	for _, want := range []string{"reports/", "notes/sample.txt", "[hierarchy]"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q\n%s", want, view)
		}
	}
}
