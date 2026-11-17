package aws

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	opensearchtypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
)

func TestOpenSearchFromStatus(t *testing.T) {
	tests := []struct {
		name            string
		in              opensearchtypes.DomainStatus
		tags            map[string]string
		tagsFetchFailed bool
		want            OpenSearchResource
	}{
		{
			name: "zero",
			in:   opensearchtypes.DomainStatus{},
			want: OpenSearchResource{State: "active"},
		},
		{
			name: "active public domain",
			in: opensearchtypes.DomainStatus{
				ARN:           aws.String("arn:aws:es:ap-northeast-1:123:domain/products"),
				DomainName:    aws.String("products"),
				EngineVersion: aws.String("OpenSearch_2.19"),
				ClusterConfig: &opensearchtypes.ClusterConfig{
					InstanceType:           opensearchtypes.OpenSearchPartitionInstanceTypeM5LargeSearch,
					InstanceCount:          aws.Int32(3),
					DedicatedMasterEnabled: aws.Bool(true),
					DedicatedMasterType:    opensearchtypes.OpenSearchPartitionInstanceTypeM5LargeSearch,
					DedicatedMasterCount:   aws.Int32(3),
				},
				EBSOptions: &opensearchtypes.EBSOptions{
					VolumeSize: aws.Int32(100),
					VolumeType: opensearchtypes.VolumeTypeGp3,
				},
				Endpoint:                aws.String("search-products.ap-northeast-1.es.amazonaws.com"),
				DomainProcessingStatus:  opensearchtypes.DomainProcessingStatusTypeActive,
				EncryptionAtRestOptions: &opensearchtypes.EncryptionAtRestOptions{Enabled: aws.Bool(true)},
				NodeToNodeEncryptionOptions: &opensearchtypes.NodeToNodeEncryptionOptions{
					Enabled: aws.Bool(true),
				},
				DomainEndpointOptions: &opensearchtypes.DomainEndpointOptions{EnforceHTTPS: aws.Bool(true)},
			},
			tags: map[string]string{"Env": "prod"},
			want: OpenSearchResource{
				ID:                     "arn:aws:es:ap-northeast-1:123:domain/products",
				Name:                   "products",
				State:                  "active",
				EngineVersion:          "OpenSearch_2.19",
				DataNodeType:           "m5.large.search",
				DataNodeCount:          3,
				DedicatedMasterEnabled: true,
				DedicatedMasterType:    "m5.large.search",
				DedicatedMasterCount:   3,
				EBSVolumeSizeGB:        100,
				EBSVolumeType:          "gp3",
				Endpoint:               "search-products.ap-northeast-1.es.amazonaws.com",
				EncryptionAtRest:       true,
				NodeToNodeEncryption:   true,
				EnforceHTTPS:           true,
				Tags:                   map[string]string{"Env": "prod"},
			},
		},
		{
			name: "vpc domain falls back to the vpc endpoint",
			in: opensearchtypes.DomainStatus{
				ARN:           aws.String("arn:aws:es:ap-northeast-1:123:domain/internal"),
				DomainName:    aws.String("internal"),
				EngineVersion: aws.String("OpenSearch_3.1"),
				ClusterConfig: &opensearchtypes.ClusterConfig{
					InstanceType:  opensearchtypes.OpenSearchPartitionInstanceTypeR6gLargeSearch,
					InstanceCount: aws.Int32(2),
				},
				Endpoints: map[string]string{"vpc": "vpc-endpoint-abc.ap-northeast-1.es.amazonaws.com"},
				VPCOptions: &opensearchtypes.VPCDerivedInfo{
					VPCId: aws.String("vpc-0123456789abcdef0"),
				},
				DomainProcessingStatus: opensearchtypes.DomainProcessingStatusTypeActive,
			},
			tags: map[string]string{},
			want: OpenSearchResource{
				ID:            "arn:aws:es:ap-northeast-1:123:domain/internal",
				Name:          "internal",
				State:         "active",
				EngineVersion: "OpenSearch_3.1",
				DataNodeType:  "r6g.large.search",
				DataNodeCount: 2,
				Endpoint:      "vpc-endpoint-abc.ap-northeast-1.es.amazonaws.com",
				VPCID:         "vpc-0123456789abcdef0",
				Tags:          map[string]string{},
			},
		},
		{
			name: "dual stack vpc domain falls back to the vpcv2 endpoint",
			in: opensearchtypes.DomainStatus{
				ARN:        aws.String("arn:aws:es:ap-northeast-1:123:domain/dualstack"),
				DomainName: aws.String("dualstack"),
				Endpoints: map[string]string{
					"vpcv2": "vpc-endpoint-def.aos.ap-northeast-1.on.aws",
				},
				DomainProcessingStatus: opensearchtypes.DomainProcessingStatusTypeActive,
			},
			tags: map[string]string{},
			want: OpenSearchResource{
				ID:       "arn:aws:es:ap-northeast-1:123:domain/dualstack",
				Name:     "dualstack",
				State:    "active",
				Endpoint: "vpc-endpoint-def.aos.ap-northeast-1.on.aws",
				Tags:     map[string]string{},
			},
		},
		{
			name: "creating flag without DomainProcessingStatus",
			in: opensearchtypes.DomainStatus{
				DomainName: aws.String("new"),
				Created:    aws.Bool(false),
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "new", State: "creating", Tags: map[string]string{}},
		},
		{
			name: "deleting flag without DomainProcessingStatus",
			in: opensearchtypes.DomainStatus{
				DomainName: aws.String("old"),
				Deleted:    aws.Bool(true),
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "old", State: "deleting", Tags: map[string]string{}},
		},
		{
			name: "processing flag without DomainProcessingStatus",
			in: opensearchtypes.DomainStatus{
				DomainName: aws.String("changing"),
				Processing: aws.Bool(true),
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "changing", State: "updating", Tags: map[string]string{}},
		},
		{
			name: "upgrade processing flag without DomainProcessingStatus",
			in: opensearchtypes.DomainStatus{
				DomainName:        aws.String("upgrading"),
				UpgradeProcessing: aws.Bool(true),
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "upgrading", State: "updating", Tags: map[string]string{}},
		},
		{
			name: "isolated domain is an error",
			in: opensearchtypes.DomainStatus{
				DomainName:             aws.String("isolated"),
				DomainProcessingStatus: opensearchtypes.DomainProcessingStatusTypeIsolated,
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "isolated", State: "error", Tags: map[string]string{}},
		},
		{
			name: "modifying domain is updating",
			in: opensearchtypes.DomainStatus{
				DomainName:             aws.String("modifying"),
				DomainProcessingStatus: opensearchtypes.DomainProcessingStatusTypeModifying,
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "modifying", State: "updating", Tags: map[string]string{}},
		},
		{
			name: "unknown processing status is normalized",
			in: opensearchtypes.DomainStatus{
				DomainName:             aws.String("future"),
				DomainProcessingStatus: opensearchtypes.DomainProcessingStatusType("Scaling"),
			},
			tags: map[string]string{},
			want: OpenSearchResource{Name: "future", State: "scaling", Tags: map[string]string{}},
		},
		{
			name: "tag fetch failure is carried",
			in: opensearchtypes.DomainStatus{
				DomainName:             aws.String("degraded"),
				DomainProcessingStatus: opensearchtypes.DomainProcessingStatusTypeActive,
			},
			tags:            map[string]string{},
			tagsFetchFailed: true,
			want: OpenSearchResource{
				Name:            "degraded",
				State:           "active",
				Tags:            map[string]string{},
				TagsFetchFailed: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openSearchFromStatus(tt.in, tt.tags, tt.tagsFetchFailed)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestOpenSearchDomainNames(t *testing.T) {
	tests := []struct {
		name string
		in   *opensearch.ListDomainNamesOutput
		want []string
	}{
		{name: "nil", in: nil, want: nil},
		{name: "empty", in: &opensearch.ListDomainNamesOutput{}, want: []string{}},
		{
			name: "empty names are dropped, duplicates removed, and the result sorted",
			in: &opensearch.ListDomainNamesOutput{
				DomainNames: []opensearchtypes.DomainInfo{
					{DomainName: aws.String("delta")},
					{DomainName: aws.String("")},
					{DomainName: aws.String("alpha")},
					{DomainName: aws.String("delta")},
					{DomainName: nil},
					{DomainName: aws.String("bravo")},
				},
			},
			want: []string{"alpha", "bravo", "delta"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openSearchDomainNames(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

// mockOpenSearchListClient は openSearchListClient の手書きモック。
// DescribeDomains と ListTags はテスト側で差し替え、受け取った Input を記録する。
type mockOpenSearchListClient struct {
	listDomainNames func(*opensearch.ListDomainNamesInput) (*opensearch.ListDomainNamesOutput, error)
	describeDomains func(*opensearch.DescribeDomainsInput) (*opensearch.DescribeDomainsOutput, error)
	listTags        func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error)

	mu             sync.Mutex
	describeInputs []*opensearch.DescribeDomainsInput
	tagInputs      []*opensearch.ListTagsInput
}

func (m *mockOpenSearchListClient) ListDomainNames(_ context.Context, params *opensearch.ListDomainNamesInput, _ ...func(*opensearch.Options)) (*opensearch.ListDomainNamesOutput, error) {
	return m.listDomainNames(params)
}

func (m *mockOpenSearchListClient) DescribeDomains(_ context.Context, params *opensearch.DescribeDomainsInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeDomainsOutput, error) {
	m.mu.Lock()
	m.describeInputs = append(m.describeInputs, params)
	m.mu.Unlock()
	return m.describeDomains(params)
}

func (m *mockOpenSearchListClient) ListTags(_ context.Context, params *opensearch.ListTagsInput, _ ...func(*opensearch.Options)) (*opensearch.ListTagsOutput, error) {
	m.mu.Lock()
	m.tagInputs = append(m.tagInputs, params)
	m.mu.Unlock()
	return m.listTags(params)
}

func TestFetchOpenSearchTags(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		arn         string
		client      *mockOpenSearchListClient
		wantTags    map[string]string
		wantFailed  bool
		wantErr     error
		wantErrText string
	}{
		{
			name: "empty ARN skips the API call",
			arn:  "",
			client: &mockOpenSearchListClient{
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					t.Fatal("ListTags must not be called when the ARN is empty")
					return nil, nil
				},
			},
			wantTags: map[string]string{},
		},
		{
			name: "success",
			arn:  "arn:aws:es:ap-northeast-1:123:domain/products",
			client: &mockOpenSearchListClient{
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					return &opensearch.ListTagsOutput{
						TagList: []opensearchtypes.Tag{{Key: aws.String("Env"), Value: aws.String("prod")}},
					}, nil
				},
			},
			wantTags: map[string]string{"Env": "prod"},
		},
		{
			name: "nil output without error is degraded",
			arn:  "arn:aws:es:ap-northeast-1:123:domain/products",
			client: &mockOpenSearchListClient{
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					return nil, nil
				},
			},
			wantTags:   map[string]string{},
			wantFailed: true,
		},
		{
			name: "API error is degraded",
			arn:  "arn:aws:es:ap-northeast-1:123:domain/products",
			client: &mockOpenSearchListClient{
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					return nil, boom
				},
			},
			wantTags:   map[string]string{},
			wantFailed: true,
		},
		{
			name: "cancellation is propagated",
			arn:  "arn:aws:es:ap-northeast-1:123:domain/products",
			client: &mockOpenSearchListClient{
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					return nil, fmt.Errorf("list tags: %w", context.Canceled)
				},
			},
			wantErr:     context.Canceled,
			wantErrText: "list tags: context canceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, failed, err := fetchOpenSearchTags(context.Background(), tt.client, tt.arn)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if err.Error() != tt.wantErrText {
					t.Errorf("err = %q, want %q", err.Error(), tt.wantErrText)
				}
				return
			}
			if failed != tt.wantFailed {
				t.Errorf("failed = %v, want %v", failed, tt.wantFailed)
			}
			if !reflect.DeepEqual(tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", tags, tt.wantTags)
			}
		})
	}
}

func TestListOpenSearchResourcesBatchesDescribeAndFetchesTags(t *testing.T) {
	names := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}
	listOut := &opensearch.ListDomainNamesOutput{}
	for _, name := range names {
		listOut.DomainNames = append(listOut.DomainNames, opensearchtypes.DomainInfo{
			DomainName: aws.String(name),
		})
	}

	tagFailure := errors.New("tag boom")
	client := &mockOpenSearchListClient{
		listDomainNames: func(*opensearch.ListDomainNamesInput) (*opensearch.ListDomainNamesOutput, error) {
			return listOut, nil
		},
		describeDomains: func(in *opensearch.DescribeDomainsInput) (*opensearch.DescribeDomainsOutput, error) {
			out := &opensearch.DescribeDomainsOutput{}
			// 応答順は API に保証が無いため、呼び出し側で整列されることを確かめるため逆順で返す。
			for i := len(in.DomainNames) - 1; i >= 0; i-- {
				name := in.DomainNames[i]
				out.DomainStatusList = append(out.DomainStatusList, opensearchtypes.DomainStatus{
					ARN:                    aws.String("arn:aws:es:ap-northeast-1:123:domain/" + name),
					DomainName:             aws.String(name),
					DomainProcessingStatus: opensearchtypes.DomainProcessingStatusTypeActive,
				})
			}
			return out, nil
		},
		listTags: func(in *opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
			if ptrStr(in.ARN) == "arn:aws:es:ap-northeast-1:123:domain/charlie" {
				return nil, tagFailure
			}
			return &opensearch.ListTagsOutput{
				TagList: []opensearchtypes.Tag{{Key: aws.String("Name"), Value: aws.String(ptrStr(in.ARN))}},
			}, nil
		},
	}

	got, err := listOpenSearchResources(context.Background(), client, "test-profile", "ap-northeast-1")
	if err != nil {
		t.Fatalf("listOpenSearchResources() error = %v", err)
	}

	// DescribeDomains は 5 件ずつの 2 回に分かれ、1 回目は Name 順の先頭 5 件になる。
	client.mu.Lock()
	describeInputs := slices.Clone(client.describeInputs)
	tagInputs := slices.Clone(client.tagInputs)
	client.mu.Unlock()

	if len(describeInputs) != 2 {
		t.Fatalf("DescribeDomains calls = %d, want 2", len(describeInputs))
	}
	if want := names[:5]; !reflect.DeepEqual(describeInputs[0].DomainNames, want) {
		t.Errorf("first DescribeDomains names = %v, want %v", describeInputs[0].DomainNames, want)
	}
	if want := names[5:]; !reflect.DeepEqual(describeInputs[1].DomainNames, want) {
		t.Errorf("second DescribeDomains names = %v, want %v", describeInputs[1].DomainNames, want)
	}

	// タグは全ドメインについて 1 回ずつ取得する。並列実行のため呼び出し順は整列して比較する。
	if len(tagInputs) != len(names) {
		t.Fatalf("ListTags calls = %d, want %d", len(tagInputs), len(names))
	}
	var gotARNs []string
	for _, in := range tagInputs {
		gotARNs = append(gotARNs, ptrStr(in.ARN))
	}
	slices.Sort(gotARNs)
	wantARNs := make([]string, 0, len(names))
	for _, name := range names {
		wantARNs = append(wantARNs, "arn:aws:es:ap-northeast-1:123:domain/"+name)
	}
	if !reflect.DeepEqual(gotARNs, wantARNs) {
		t.Errorf("ListTags ARNs = %v, want %v", gotARNs, wantARNs)
	}

	// 返却は Name 順。タグ取得に失敗したドメインだけ TagsFetchFailed が立つ。
	var gotNames []string
	for _, r := range got {
		gotNames = append(gotNames, r.Name)
	}
	if !reflect.DeepEqual(gotNames, names) {
		t.Errorf("resource names = %v, want %v", gotNames, names)
	}
	for _, r := range got {
		wantFailed := r.Name == "charlie"
		if r.TagsFetchFailed != wantFailed {
			t.Errorf("%s TagsFetchFailed = %v, want %v", r.Name, r.TagsFetchFailed, wantFailed)
		}
		if wantFailed {
			if len(r.Tags) != 0 {
				t.Errorf("%s Tags = %v, want empty on failure", r.Name, r.Tags)
			}
			continue
		}
		wantTags := map[string]string{"Name": r.ID}
		if !reflect.DeepEqual(r.Tags, wantTags) {
			t.Errorf("%s Tags = %v, want %v", r.Name, r.Tags, wantTags)
		}
	}
}

func TestListOpenSearchResourcesEmptyList(t *testing.T) {
	client := &mockOpenSearchListClient{
		listDomainNames: func(*opensearch.ListDomainNamesInput) (*opensearch.ListDomainNamesOutput, error) {
			return &opensearch.ListDomainNamesOutput{}, nil
		},
		describeDomains: func(*opensearch.DescribeDomainsInput) (*opensearch.DescribeDomainsOutput, error) {
			t.Fatal("DescribeDomains must not be called for an empty domain list")
			return nil, nil
		},
		listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
			t.Fatal("ListTags must not be called for an empty domain list")
			return nil, nil
		},
	}
	got, err := listOpenSearchResources(context.Background(), client, "p", "ap-northeast-1")
	if err != nil {
		t.Fatalf("listOpenSearchResources() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got = %#v, want an empty non-nil slice", got)
	}
}

func TestListOpenSearchResourcesErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		client  *mockOpenSearchListClient
		wantErr string
		target  error
	}{
		{
			name: "list domain names",
			client: &mockOpenSearchListClient{
				listDomainNames: func(*opensearch.ListDomainNamesInput) (*opensearch.ListDomainNamesOutput, error) {
					return nil, boom
				},
			},
			wantErr: "list opensearch domains: boom",
			target:  boom,
		},
		{
			name: "describe domains",
			client: &mockOpenSearchListClient{
				listDomainNames: func(*opensearch.ListDomainNamesInput) (*opensearch.ListDomainNamesOutput, error) {
					return &opensearch.ListDomainNamesOutput{
						DomainNames: []opensearchtypes.DomainInfo{{DomainName: aws.String("alpha")}},
					}, nil
				},
				describeDomains: func(*opensearch.DescribeDomainsInput) (*opensearch.DescribeDomainsOutput, error) {
					return nil, boom
				},
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					t.Fatal("ListTags must not be called when DescribeDomains fails")
					return nil, nil
				},
			},
			wantErr: "describe opensearch domains [alpha]: boom",
			target:  boom,
		},
		{
			name: "cancelled tag fetch is propagated",
			client: &mockOpenSearchListClient{
				listDomainNames: func(*opensearch.ListDomainNamesInput) (*opensearch.ListDomainNamesOutput, error) {
					return &opensearch.ListDomainNamesOutput{
						DomainNames: []opensearchtypes.DomainInfo{{DomainName: aws.String("alpha")}},
					}, nil
				},
				describeDomains: func(in *opensearch.DescribeDomainsInput) (*opensearch.DescribeDomainsOutput, error) {
					return &opensearch.DescribeDomainsOutput{
						DomainStatusList: []opensearchtypes.DomainStatus{{
							ARN:        aws.String("arn:aws:es:ap-northeast-1:123:domain/alpha"),
							DomainName: aws.String("alpha"),
						}},
					}, nil
				},
				listTags: func(*opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error) {
					return nil, fmt.Errorf("list tags: %w", context.Canceled)
				},
			},
			wantErr: "list tags: context canceled",
			target:  context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := listOpenSearchResources(context.Background(), tt.client, "p", "ap-northeast-1")
			if !errors.Is(err, tt.target) {
				t.Fatalf("err = %v, want wrapped %v", err, tt.target)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("err = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestOpenSearchResourceToRow(t *testing.T) {
	r := OpenSearchResource{
		Name:          "products",
		State:         "active",
		EngineVersion: "OpenSearch_2.19",
		DataNodeType:  "or2.large.search",
		DataNodeCount: 3,
		Endpoint:      "search-products.ap-northeast-1.es.amazonaws.com",
	}
	want := []string{
		"products",
		"active",
		"OpenSearch_2.19",
		"or2.large.search",
		"3",
		"search-products.ap-northeast-1.es.amazonaws.com",
	}
	if got := r.ToRow(); !reflect.DeepEqual(got, want) {
		t.Errorf("ToRow() = %v, want %v", got, want)
	}
}
