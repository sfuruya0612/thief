package aws

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

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
