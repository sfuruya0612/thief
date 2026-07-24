package aws

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// mockRDSClusterParameterClient は listRDSClusterParameters が要求する
// rdsClusterParameterClient をテスト用に実装する手書きモック。
type mockRDSClusterParameterClient struct {
	describeDBClusters           func(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	describeDBClusterParameters  func(ctx context.Context, params *rds.DescribeDBClusterParametersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClusterParametersOutput, error)
	describeDBClusterParamsCalls []string
}

func (m *mockRDSClusterParameterClient) DescribeDBClusters(ctx context.Context, params *rds.DescribeDBClustersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	return m.describeDBClusters(ctx, params, optFns...)
}

func (m *mockRDSClusterParameterClient) DescribeDBClusterParameters(ctx context.Context, params *rds.DescribeDBClusterParametersInput, optFns ...func(*rds.Options)) (*rds.DescribeDBClusterParametersOutput, error) {
	m.describeDBClusterParamsCalls = append(m.describeDBClusterParamsCalls, aws.ToString(params.DBClusterParameterGroupName))
	return m.describeDBClusterParameters(ctx, params, optFns...)
}

func TestRdsFromInstanceParameterGroups(t *testing.T) {
	tests := []struct {
		name string
		in   []rdstypes.DBParameterGroupStatus
		want []string
	}{
		{
			name: "single group",
			in:   []rdstypes.DBParameterGroupStatus{{DBParameterGroupName: aws.String("default.mysql8.0")}},
			want: []string{"default.mysql8.0"},
		},
		{
			name: "multiple groups",
			in: []rdstypes.DBParameterGroupStatus{
				{DBParameterGroupName: aws.String("pg-a")},
				{DBParameterGroupName: aws.String("pg-b")},
			},
			want: []string{"pg-a", "pg-b"},
		},
		{
			name: "nil name skipped",
			in: []rdstypes.DBParameterGroupStatus{
				{DBParameterGroupName: aws.String("pg-a")},
				{DBParameterGroupName: nil},
			},
			want: []string{"pg-a"},
		},
		{
			name: "no groups",
			in:   nil,
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DBSubnetGroup は rdsFromInstance が VpcId を参照するため必ず設定する。
			in := rdstypes.DBInstance{
				DBInstanceIdentifier: aws.String("db-1"),
				DBSubnetGroup:        &rdstypes.DBSubnetGroup{VpcId: aws.String("vpc-1")},
				DBParameterGroups:    tt.in,
			}
			got := rdsFromInstance(in).ParameterGroups
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestRdsFromInstanceClusterID(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want string
	}{
		{
			name: "belongs to cluster",
			in:   aws.String("aurora-cluster-1"),
			want: "aurora-cluster-1",
		},
		{
			name: "not part of a cluster",
			in:   nil,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := rdstypes.DBInstance{
				DBInstanceIdentifier: aws.String("db-1"),
				DBSubnetGroup:        &rdstypes.DBSubnetGroup{VpcId: aws.String("vpc-1")},
				DBClusterIdentifier:  tt.in,
			}
			got := rdsFromInstance(in).ClusterID
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestListRDSClusterParameters(t *testing.T) {
	t.Run("DescribeDBClusters で得たグループ名を DescribeDBClusterParameters へ渡す", func(t *testing.T) {
		client := &mockRDSClusterParameterClient{
			describeDBClusters: func(_ context.Context, params *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
				if aws.ToString(params.DBClusterIdentifier) != "aurora-cluster-1" {
					t.Errorf("DBClusterIdentifier = %q, want %q", aws.ToString(params.DBClusterIdentifier), "aurora-cluster-1")
				}
				return &rds.DescribeDBClustersOutput{
					DBClusters: []rdstypes.DBCluster{
						{DBClusterParameterGroup: aws.String("default.aurora-mysql8.0")},
					},
				}, nil
			},
			describeDBClusterParameters: func(_ context.Context, _ *rds.DescribeDBClusterParametersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClusterParametersOutput, error) {
				return &rds.DescribeDBClusterParametersOutput{
					Parameters: []rdstypes.Parameter{
						{ParameterName: aws.String("binlog_format"), ParameterValue: aws.String("ROW")},
					},
				}, nil
			},
		}

		got, err := listRDSClusterParameters(context.Background(), client, "aurora-cluster-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []RDSParameter{{Name: "binlog_format", Value: "ROW"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v want %#v", got, want)
		}
		if !reflect.DeepEqual(client.describeDBClusterParamsCalls, []string{"default.aurora-mysql8.0"}) {
			t.Errorf("DescribeDBClusterParameters group name calls = %v", client.describeDBClusterParamsCalls)
		}
	})

	t.Run("複数ページのパラメータをすべて集約する", func(t *testing.T) {
		pages := []*rds.DescribeDBClusterParametersOutput{
			{
				Parameters: []rdstypes.Parameter{{ParameterName: aws.String("p1")}},
				Marker:     aws.String("next"),
			},
			{
				Parameters: []rdstypes.Parameter{{ParameterName: aws.String("p2")}},
			},
		}
		call := 0
		client := &mockRDSClusterParameterClient{
			describeDBClusters: func(_ context.Context, _ *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
				return &rds.DescribeDBClustersOutput{
					DBClusters: []rdstypes.DBCluster{{DBClusterParameterGroup: aws.String("pg-cluster")}},
				}, nil
			},
			describeDBClusterParameters: func(_ context.Context, _ *rds.DescribeDBClusterParametersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClusterParametersOutput, error) {
				page := pages[call]
				call++
				return page, nil
			},
		}

		got, err := listRDSClusterParameters(context.Background(), client, "aurora-cluster-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []RDSParameter{{Name: "p1"}, {Name: "p2"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v want %#v", got, want)
		}
	})

	t.Run("DescribeDBClusters の失敗をエラーとして伝播する", func(t *testing.T) {
		wantErr := errors.New("throttled")
		client := &mockRDSClusterParameterClient{
			describeDBClusters: func(_ context.Context, _ *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
				return nil, wantErr
			},
		}

		_, err := listRDSClusterParameters(context.Background(), client, "aurora-cluster-1")
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapping %v", err, wantErr)
		}
	})

	t.Run("クラスターが見つからない場合はエラーを返す", func(t *testing.T) {
		client := &mockRDSClusterParameterClient{
			describeDBClusters: func(_ context.Context, _ *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
				return &rds.DescribeDBClustersOutput{}, nil
			},
		}

		_, err := listRDSClusterParameters(context.Background(), client, "missing-cluster")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("DescribeDBClusterParameters の失敗をエラーとして伝播する", func(t *testing.T) {
		wantErr := errors.New("access denied")
		client := &mockRDSClusterParameterClient{
			describeDBClusters: func(_ context.Context, _ *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
				return &rds.DescribeDBClustersOutput{
					DBClusters: []rdstypes.DBCluster{{DBClusterParameterGroup: aws.String("pg-cluster")}},
				}, nil
			},
			describeDBClusterParameters: func(_ context.Context, _ *rds.DescribeDBClusterParametersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClusterParametersOutput, error) {
				return nil, wantErr
			},
		}

		_, err := listRDSClusterParameters(context.Background(), client, "aurora-cluster-1")
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapping %v", err, wantErr)
		}
	})
}

func TestRdsParameterFromSDK(t *testing.T) {
	tests := []struct {
		name string
		in   rdstypes.Parameter
		want RDSParameter
	}{
		{
			name: "populated",
			in: rdstypes.Parameter{
				ParameterName:  aws.String("max_connections"),
				ParameterValue: aws.String("100"),
				AllowedValues:  aws.String("1-16384"),
				ApplyType:      aws.String("dynamic"),
				DataType:       aws.String("integer"),
				Source:         aws.String("user"),
				IsModifiable:   aws.Bool(true),
				Description:    aws.String("maximum number of connections"),
			},
			want: RDSParameter{
				Name:          "max_connections",
				Value:         "100",
				AllowedValues: "1-16384",
				ApplyType:     "dynamic",
				DataType:      "integer",
				Source:        "user",
				IsModifiable:  true,
				Description:   "maximum number of connections",
			},
		},
		{
			name: "nil pointers become empty",
			in:   rdstypes.Parameter{ParameterName: aws.String("p")},
			want: RDSParameter{Name: "p"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rdsParameterFromSDK(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}
