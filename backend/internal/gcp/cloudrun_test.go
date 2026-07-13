package gcp

import (
	"testing"
	"time"

	"cloud.google.com/go/run/apiv2/runpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRegionFromResourceName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "service", in: "projects/proj/locations/asia-northeast1/services/hello", want: "asia-northeast1"},
		{name: "job", in: "projects/proj/locations/us-central1/jobs/nightly", want: "us-central1"},
		{name: "trailing_location", in: "projects/proj/locations/europe-west1", want: "europe-west1"},
		{name: "no_locations_segment", in: "projects/proj/services/hello", want: ""},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := regionFromResourceName(tt.in)
			if got != tt.want {
				t.Fatalf("regionFromResourceName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunResourceFromService(t *testing.T) {
	ct := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ut := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	svc := &runpb.Service{
		Name:       "projects/proj/locations/asia-northeast1/services/hello",
		Uri:        "https://hello-abc.a.run.app",
		CreateTime: timestamppb.New(ct),
		UpdateTime: timestamppb.New(ut),
	}
	want := RunResourceInfo{
		Name:       "projects/proj/locations/asia-northeast1/services/hello",
		Kind:       "service",
		Region:     "asia-northeast1",
		URI:        "https://hello-abc.a.run.app",
		CreateTime: ct.Format(time.RFC3339),
		UpdateTime: ut.Format(time.RFC3339),
	}
	got := runResourceFromService(svc)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	// nil safety
	if got := runResourceFromService(nil); got.Kind != "service" {
		t.Errorf("nil service: Kind = %q, want %q", got.Kind, "service")
	}
}

func TestRunResourceFromJob(t *testing.T) {
	ct := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	ut := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	job := &runpb.Job{
		Name:       "projects/proj/locations/us-central1/jobs/nightly",
		CreateTime: timestamppb.New(ct),
		UpdateTime: timestamppb.New(ut),
	}
	want := RunResourceInfo{
		Name:       "projects/proj/locations/us-central1/jobs/nightly",
		Kind:       "job",
		Region:     "us-central1",
		CreateTime: ct.Format(time.RFC3339),
		UpdateTime: ut.Format(time.RFC3339),
	}
	got := runResourceFromJob(job)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	if got := runResourceFromJob(nil); got.Kind != "job" {
		t.Errorf("nil job: Kind = %q, want %q", got.Kind, "job")
	}
}
