package aws

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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

// mockECRDescribeImagesClient は listECRImageInfos が要求する ecrDescribeImagesClient を
// テスト用に実装する手書きモック。受け取った Input を呼び出し順に記録し、pages に用意した
// レスポンスを 1 呼び出しにつき 1 ページ返す。
// listECRImageInfos の自前ループはページごとに Input を新しく確保するため、記録した後に
// 内容が書き換わることはなく、ポインタのまま記録してよい。
type mockECRDescribeImagesClient struct {
	pages  []*ecr.DescribeImagesOutput
	inputs []*ecr.DescribeImagesInput
}

func (m *mockECRDescribeImagesClient) DescribeImages(_ context.Context, params *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	m.inputs = append(m.inputs, params)
	idx := len(m.inputs) - 1
	if idx >= len(m.pages) {
		return nil, fmt.Errorf("unexpected DescribeImages call %d: only %d pages prepared", idx+1, len(m.pages))
	}
	return m.pages[idx], nil
}

// ecrDescribeImagesPages は NextToken で連結された 2 ページ構成のレスポンスを返す。
// 1 ページ目の ImagePushedAt を 2 ページ目より古くしてあるため、両ページを集約したうえで
// PushedAt 降順に並べると 2 ページ目のイメージが先に来る。
func ecrDescribeImagesPages() []*ecr.DescribeImagesOutput {
	return []*ecr.DescribeImagesOutput{
		{
			ImageDetails: []ecrtypes.ImageDetail{
				{
					ImageDigest:   aws.String("sha256:page1"),
					ImagePushedAt: aws.Time(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
			},
			NextToken: aws.String("page-2"),
		},
		{
			ImageDetails: []ecrtypes.ImageDetail{
				{
					ImageDigest:   aws.String("sha256:page2"),
					ImagePushedAt: aws.Time(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
				},
			},
		},
	}
}

// TestListECRImageInfosSendsFilterAndMaxResults は all の分岐ごとに、DescribeImages へ送られる
// Input を検証する。Filter を落とすとタグなしイメージが混ざり、MaxResults を落とすと AWS 既定の
// ページサイズになるが、いずれも呼び出しは成功してしまうため Input を直接固定する。
// 期待値は実装の定数を参照せず、件数 (1000 / 30) と TagStatus を直接書き下している。
func TestListECRImageInfosSendsFilterAndMaxResults(t *testing.T) {
	const repoName = "app-repo"

	tests := []struct {
		name string
		all  bool
		// wantInputs は呼び出し順に期待する Input。要素数が期待する呼び出し回数を兼ねる。
		wantInputs  []*ecr.DescribeImagesInput
		wantDigests []string
	}{
		{
			// all=true はタグ絞り込みを行わず、NextToken が尽きるまでページを辿る。
			name: "all=true は Filter なしで全ページを辿る",
			all:  true,
			wantInputs: []*ecr.DescribeImagesInput{
				{
					RepositoryName: aws.String(repoName),
					MaxResults:     aws.Int32(1000),
				},
				{
					RepositoryName: aws.String(repoName),
					MaxResults:     aws.Int32(1000),
					NextToken:      aws.String("page-2"),
				},
			},
			wantDigests: []string{"sha256:page2", "sha256:page1"},
		},
		{
			// all=false はタグ付きのみに絞り、NextToken が返っても先頭ページで打ち切る。
			name: "all=false は Filter 付きで先頭ページのみ",
			all:  false,
			wantInputs: []*ecr.DescribeImagesInput{
				{
					RepositoryName: aws.String(repoName),
					MaxResults:     aws.Int32(30),
					Filter:         &ecrtypes.DescribeImagesFilter{TagStatus: ecrtypes.TagStatusTagged},
				},
			},
			wantDigests: []string{"sha256:page1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockECRDescribeImagesClient{pages: ecrDescribeImagesPages()}
			got, err := listECRImageInfos(context.Background(), client, repoName, tt.all)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(client.inputs) != len(tt.wantInputs) {
				t.Fatalf("DescribeImages called %d times, want %d", len(client.inputs), len(tt.wantInputs))
			}
			// Input 全体を比較することで、MaxResults と Filter に加えて RepositoryName と
			// NextToken の引き継ぎも同時に固定する。
			opts := cmpopts.IgnoreUnexported(ecr.DescribeImagesInput{}, ecrtypes.DescribeImagesFilter{})
			for i, in := range client.inputs {
				if diff := cmp.Diff(tt.wantInputs[i], in, opts); diff != "" {
					t.Errorf("call %d: input mismatch (-want +got):\n%s", i+1, diff)
				}
			}

			// 取得したページのイメージが集約されることも確認する。all=false で 2 ページ目の
			// イメージが混ざっていないこと、all=true で 1 ページ目が捨てられていないことを
			// ここで検出する。並び順は PushedAt 降順。
			gotDigests := make([]string, 0, len(got))
			for _, img := range got {
				gotDigests = append(gotDigests, img.ImageDigest)
			}
			if diff := cmp.Diff(tt.wantDigests, gotDigests); diff != "" {
				t.Errorf("image digests mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
