package aws

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

func TestEcrImageFromDetail(t *testing.T) {
	pushedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pulledAt := time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC)

	tests := []struct {
		name     string
		repoName string
		in       ecrtypes.ImageDetail
		want     ECRImageResource
	}{
		{
			name:     "pushed and pulled populated",
			repoName: "my-repo",
			in: ecrtypes.ImageDetail{
				ImageTags:            []string{"latest"},
				ImageDigest:          aws.String("sha256:abc"),
				ImagePushedAt:        &pushedAt,
				LastRecordedPullTime: &pulledAt,
				ImageSizeInBytes:     aws.Int64(1024),
			},
			want: ECRImageResource{
				RepositoryName: "my-repo",
				ImageTag:       "latest",
				ImageDigest:    "sha256:abc",
				PushedAt:       pushedAt.Format(time.RFC3339),
				LastPulledAt:   pulledAt.Format(time.RFC3339),
				ImageSizeBytes: 1024,
			},
		},
		{
			name:     "never pulled stays empty",
			repoName: "my-repo",
			in: ecrtypes.ImageDetail{
				ImageDigest:   aws.String("sha256:def"),
				ImagePushedAt: &pushedAt,
			},
			want: ECRImageResource{
				RepositoryName: "my-repo",
				ImageDigest:    "sha256:def",
				PushedAt:       pushedAt.Format(time.RFC3339),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ecrImageFromDetail(tt.repoName, tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
