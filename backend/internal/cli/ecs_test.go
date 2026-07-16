package cli

import "testing"

func TestArnName(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		num  int
		want string
	}{
		{
			name: "cluster name from cluster arn",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
			num:  1,
			want: "my-cluster",
		},
		{
			name: "task id from task arn",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:task/my-cluster/abcdef1234567890",
			num:  2,
			want: "abcdef1234567890",
		},
		{
			name: "out of range returns full arn",
			arn:  "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
			num:  5,
			want: "arn:aws:ecs:ap-northeast-1:123456789012:cluster/my-cluster",
		},
		{
			name: "negative index returns full arn",
			arn:  "a/b",
			num:  -1,
			want: "a/b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arnName(tt.arn, tt.num); got != tt.want {
				t.Errorf("arnName(%q, %d) = %q, want %q", tt.arn, tt.num, got, tt.want)
			}
		})
	}
}
