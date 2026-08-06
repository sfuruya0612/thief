package cli

import (
	"testing"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
)

// 本テストは internal/cli の各コマンドが使う列定義 ([]util.Column) と、対応する型の
// ToRow() が返す値の要素数と順序の対応を検証する。フィクスチャは ToRow() が参照する
// 全フィールドに相異なる値を入れ、各値が期待する列位置に現れることを確認する。
//
// 対象型は次の 3 経路の grep で列挙した (issue 0100)。
//
//  1. grep -rn 'ListConfig\[' backend/internal/cli (runList 経由の一覧表示)
//     → IAMUserInfo, SSOAccountResource, RDSInstanceInfo, RDSClusterInfo,
//     RDSParameterInfo, SecretInfo, ElastiCacheClusterInfo, ElastiCacheParameterInfo,
//     KinesisResource, LogGroupInfo, LambdaResource, ELBResource, CloudFrontResource,
//     CfnStackSummary, ECSClusterInfo, S3BucketInfo, ECRRepoInfo, ECRImageInfo,
//     SSMParameterInfo, CostResource, ForecastResource (21 型)
//  2. grep -rn 'toRows(' backend/internal/cli (runList を経由しない直接呼び出し)
//     → EC2InstanceInfo, CfnParameter, CfnOutput, CfnTag, CfnChangeDetail,
//     ECSServiceInfo, ECSTaskInfo (7 型)
//  3. grep -rn '.ToRow()' backend/internal/cli (単一項目の表示)
//     → SSMParameterValue (1 型)
//
// 非 AWS (BigQuery / Datadog / TiDB) の列定義と、internal/aws/torow.go にあるが
// internal/cli から到達しない ToRow メソッドは対象外とする (issue 0100 の完了条件)。

// colValue は 1 列分の期待値 (列ヘッダと ToRow() の値) の組。
type colValue struct {
	header string
	value  string
}

func TestColumnsToRowOrder(t *testing.T) {
	tests := []struct {
		name    string
		columns []util.Column
		row     []string
		want    []colValue
	}{
		{
			name:    "iamUserColumns / IAMUserInfo",
			columns: iamUserColumns,
			row: awsinternal.IAMUserInfo{
				UserName:   "user-1",
				UserID:     "AIDAEXAMPLE",
				Groups:     "g1,g2",
				Policies:   "p1,p2",
				CreateDate: "2026-01-01",
			}.ToRow(),
			want: []colValue{
				{"UserName", "user-1"},
				{"UserID", "AIDAEXAMPLE"},
				{"Groups", "g1,g2"},
				{"Policies", "p1,p2"},
				{"CreateDate", "2026-01-01"},
			},
		},
		{
			name:    "ssoAccountColumns / SSOAccountResource",
			columns: ssoAccountColumns,
			row: awsinternal.SSOAccountResource{
				ID:           "111122223333",
				Name:         "acct-1",
				EmailAddress: "acct@example.com",
				Roles:        []string{"Admin", "ReadOnly"},
			}.ToRow(),
			want: []colValue{
				{"ID", "111122223333"},
				{"Name", "acct-1"},
				{"Email", "acct@example.com"},
				{"Roles", "Admin,ReadOnly"},
			},
		},
		{
			name:    "rdsInstanceColumns / RDSInstanceInfo",
			columns: rdsInstanceColumns,
			row: awsinternal.RDSInstanceInfo{
				Name:            "db-1",
				DBInstanceClass: "db.t3.micro",
				Engine:          "mysql",
				EngineVersion:   "8.0.42",
				Storage:         "20GB",
				StorageType:     "gp3",
				Status:          "available",
			}.ToRow(),
			want: []colValue{
				{"Name", "db-1"},
				{"DBInstanceClass", "db.t3.micro"},
				{"Engine", "mysql"},
				{"EngineVersion", "8.0.42"},
				{"Storage", "20GB"},
				{"StorageType", "gp3"},
				{"DBInstanceStatus", "available"},
			},
		},
		{
			name:    "rdsClusterColumns / RDSClusterInfo",
			columns: rdsClusterColumns,
			row: awsinternal.RDSClusterInfo{
				Name:          "cluster-1",
				Engine:        "aurora-mysql",
				EngineVersion: "8.0.mysql_aurora.3",
				EngineMode:    "provisioned",
				Status:        "available",
			}.ToRow(),
			want: []colValue{
				{"Name", "cluster-1"},
				{"Engine", "aurora-mysql"},
				{"EngineVersion", "8.0.mysql_aurora.3"},
				{"EngineMode", "provisioned"},
				{"Status", "available"},
			},
		},
		{
			name:    "rdsParameterColumns / RDSParameterInfo",
			columns: rdsParameterColumns,
			row: awsinternal.RDSParameterInfo{
				Name:         "max_connections",
				Value:        "100",
				ApplyType:    "dynamic",
				DataType:     "integer",
				IsModifiable: "true",
				Source:       "user",
			}.ToRow(),
			want: []colValue{
				{"Name", "max_connections"},
				{"Value", "100"},
				{"ApplyType", "dynamic"},
				{"DataType", "integer"},
				{"IsModifiable", "true"},
				{"Source", "user"},
			},
		},
		{
			name:    "secretListColumns / SecretInfo",
			columns: secretListColumns,
			row: awsinternal.SecretInfo{
				Name:        "secret-1",
				Description: "for testing",
				LastChanged: "2026-01-02",
			}.ToRow(),
			want: []colValue{
				{"Name", "secret-1"},
				{"Description", "for testing"},
				{"LastChanged", "2026-01-02"},
			},
		},
		{
			name:    "elasticacheColumns / ElastiCacheClusterInfo",
			columns: elasticacheColumns,
			row: awsinternal.ElastiCacheClusterInfo{
				ReplicationGroupID:    "rg-1",
				CacheClusterID:        "cc-1",
				CacheNodeType:         "cache.t3.micro",
				Engine:                "redis",
				EngineVersion:         "7.1",
				Status:                "available",
				NodeAvailabilityZones: []string{"ap-northeast-1a", "ap-northeast-1c"},
			}.ToRow(),
			want: []colValue{
				{"ReplicationGroupId", "rg-1"},
				{"CacheClusterId", "cc-1"},
				{"CacheNodeType", "cache.t3.micro"},
				{"Engine", "redis"},
				{"EngineVersion", "7.1"},
				{"CacheClusterStatus", "available"},
				{"NodeAvailabilityZones", "ap-northeast-1a,ap-northeast-1c"},
			},
		},
		{
			name:    "elasticacheParameterColumns / ElastiCacheParameterInfo",
			columns: elasticacheParameterColumns,
			row: awsinternal.ElastiCacheParameterInfo{
				Name:         "maxmemory-policy",
				Value:        "noeviction",
				ChangeType:   "immediate",
				DataType:     "string",
				IsModifiable: "true",
				Source:       "user",
			}.ToRow(),
			want: []colValue{
				{"Name", "maxmemory-policy"},
				{"Value", "noeviction"},
				{"ChangeType", "immediate"},
				{"DataType", "string"},
				{"IsModifiable", "true"},
				{"Source", "user"},
			},
		},
		{
			name:    "kinesisColumns / KinesisResource",
			columns: kinesisColumns,
			row: awsinternal.KinesisResource{
				Name:           "stream-1",
				State:          "ACTIVE",
				Mode:           "on-demand",
				ShardCount:     4,
				RetentionHours: 24,
				EncryptionType: "KMS",
			}.ToRow(),
			want: []colValue{
				{"Name", "stream-1"},
				{"State", "ACTIVE"},
				{"Mode", "on-demand"},
				{"Shards", "4"},
				{"Retention(h)", "24"},
				{"Encryption", "KMS"},
			},
		},
		{
			name:    "cwLogsGroupColumns / LogGroupInfo",
			columns: cwLogsGroupColumns,
			row: awsinternal.LogGroupInfo{
				Name:          "/aws/lambda/fn",
				StoredBytes:   2048,
				RetentionDays: 30,
				CreationTime:  "2026-01-03",
			}.ToRow(),
			want: []colValue{
				{"Name", "/aws/lambda/fn"},
				{"StoredBytes", "2048"},
				{"RetentionDays", "30"},
				{"CreationTime", "2026-01-03"},
			},
		},
		{
			name:    "lambdaColumns / LambdaResource",
			columns: lambdaColumns,
			row: awsinternal.LambdaResource{
				Name:       "fn-1",
				State:      "Active",
				Runtime:    "provided.al2023",
				MemoryMB:   128,
				TimeoutSec: 30,
			}.ToRow(),
			want: []colValue{
				{"Name", "fn-1"},
				{"State", "Active"},
				{"Runtime", "provided.al2023"},
				{"Memory(MB)", "128"},
				{"Timeout(s)", "30"},
			},
		},
		{
			name:    "elbColumns / ELBResource",
			columns: elbColumns,
			row: awsinternal.ELBResource{
				Name:    "lb-1",
				Type:    "application",
				State:   "active",
				Scheme:  "internet-facing",
				DNSName: "lb-1.example.com",
				VpcID:   "vpc-123",
				AZs:     []string{"ap-northeast-1a", "ap-northeast-1c"},
			}.ToRow(),
			want: []colValue{
				{"Name", "lb-1"},
				{"Type", "application"},
				{"State", "active"},
				{"Scheme", "internet-facing"},
				{"DNS", "lb-1.example.com"},
				{"VPC", "vpc-123"},
				{"AZs", "ap-northeast-1a,ap-northeast-1c"},
			},
		},
		{
			name:    "cloudfrontColumns / CloudFrontResource",
			columns: cloudfrontColumns,
			row: awsinternal.CloudFrontResource{
				ID:         "E1234567890",
				Name:       "dist-1",
				State:      "Deployed",
				DomainName: "d123.cloudfront.net",
				Origins:    []string{"origin-1", "origin-2"},
				Enabled:    true,
			}.ToRow(),
			want: []colValue{
				{"ID", "E1234567890"},
				{"Name", "dist-1"},
				{"State", "Deployed"},
				{"Domain", "d123.cloudfront.net"},
				{"Origins", "origin-1,origin-2"},
				{"Enabled", "true"},
			},
		},
		{
			name:    "cfnStackColumns / CfnStackSummary",
			columns: cfnStackColumns,
			row: awsinternal.CfnStackSummary{
				StackName:   "stack-1",
				Status:      "CREATE_COMPLETE",
				DriftStatus: "IN_SYNC",
				CreatedTime: "2026-01-04",
				UpdatedTime: "2026-01-05",
				Description: "stack description",
			}.ToRow(),
			want: []colValue{
				{"StackName", "stack-1"},
				{"Status", "CREATE_COMPLETE"},
				{"DriftStatus", "IN_SYNC"},
				{"CreatedTime", "2026-01-04"},
				{"UpdatedTime", "2026-01-05"},
				{"Description", "stack description"},
			},
		},
		{
			name:    "ecsClusterColumns / ECSClusterInfo",
			columns: ecsClusterColumns,
			row: awsinternal.ECSClusterInfo{
				Name:                         "cluster-1",
				Status:                       "ACTIVE",
				ActiveServicesCount:          1,
				RunningTasksCount:            2,
				PendingTasksCount:            3,
				RegisteredContainerInstances: 4,
			}.ToRow(),
			want: []colValue{
				{"ClusterName", "cluster-1"},
				{"Status", "ACTIVE"},
				{"ActiveServices", "1"},
				{"RunningTasks", "2"},
				{"PendingTasks", "3"},
				{"RegisteredContainerInstances", "4"},
			},
		},
		{
			name:    "s3Columns / S3BucketInfo",
			columns: s3Columns,
			row: awsinternal.S3BucketInfo{
				Name:         "bucket-1",
				CreationDate: "2026-01-06",
			}.ToRow(),
			want: []colValue{
				{"BucketName", "bucket-1"},
				{"CreationDate", "2026-01-06"},
			},
		},
		{
			name:    "ecrRepoColumns / ECRRepoInfo",
			columns: ecrRepoColumns,
			row: awsinternal.ECRRepoInfo{
				RepositoryName: "repo-1",
				RepositoryUri:  "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/repo-1",
				CreatedAt:      "2026-01-07",
			}.ToRow(),
			want: []colValue{
				{"RepositoryName", "repo-1"},
				{"RepositoryUri", "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/repo-1"},
				{"CreatedAt", "2026-01-07"},
			},
		},
		{
			name:    "ecrImageColumns / ECRImageInfo",
			columns: ecrImageColumns,
			row: awsinternal.ECRImageInfo{
				RepositoryName: "repo-1",
				ImageTag:       "latest",
				ImageDigest:    "sha256:abcdef",
				PushedAt:       "2026-01-08",
				LastPulledAt:   "2026-01-09",
				ImageSizeBytes: "1024",
			}.ToRow(),
			want: []colValue{
				{"RepositoryName", "repo-1"},
				{"ImageTag", "latest"},
				{"ImageDigest", "sha256:abcdef"},
				{"PushedAt", "2026-01-08"},
				{"LastPulledAt", "2026-01-09"},
				{"ImageSizeBytes", "1024"},
			},
		},
		{
			name:    "ssmParamListColumns / SSMParameterInfo",
			columns: ssmParamListColumns,
			row: awsinternal.SSMParameterInfo{
				Name:             "/app/param",
				Type:             "String",
				LastModifiedDate: "2026-01-10",
				Version:          2,
				DataType:         "text",
			}.ToRow(),
			want: []colValue{
				{"Name", "/app/param"},
				{"Type", "String"},
				{"LastModifiedDate", "2026-01-10"},
				{"Version", "2"},
				{"DataType", "text"},
			},
		},
		{
			name:    "costColumns / CostResource",
			columns: costColumns,
			row: awsinternal.CostResource{
				TimePeriod:         "2026-01-01",
				Service:            "Amazon Elastic Compute Cloud - Compute",
				UnblendedAmount:    12.3456,
				NetAmortizedAmount: 7.89,
				Unit:               "USD",
			}.ToRow(),
			want: []colValue{
				{"Date", "2026-01-01"},
				{"Service", "Amazon Elastic Compute Cloud - Compute"},
				{"Unblended", "12.3456"},
				{"NetAmortized", "7.8900"},
				{"Unit", "USD"},
			},
		},
		{
			name:    "forecastColumns / ForecastResource",
			columns: forecastColumns,
			row: awsinternal.ForecastResource{
				TimePeriod: "2026-01-01 - 2026-01-31",
				Amount:     100.5,
				Unit:       "USD",
			}.ToRow(),
			want: []colValue{
				{"Period", "2026-01-01 - 2026-01-31"},
				{"Amount", "100.5000"},
				{"Unit", "USD"},
			},
		},
		{
			name:    "ec2Columns / EC2InstanceInfo",
			columns: ec2Columns,
			row: awsinternal.EC2InstanceInfo{
				Name:         "web-1",
				InstanceID:   "i-0123456789abcdef0",
				InstanceType: "t3.micro",
				Lifecycle:    "spot",
				PrivateIP:    "10.0.0.1",
				PublicIP:     "203.0.113.1",
				State:        "running",
				KeyName:      "key-1",
				AZ:           "ap-northeast-1a",
				LaunchTime:   "2026-01-11",
			}.ToRow(),
			want: []colValue{
				{"Name", "web-1"},
				{"InstanceID", "i-0123456789abcdef0"},
				{"InstanceType", "t3.micro"},
				{"Lifecycle", "spot"},
				{"PrivateIP", "10.0.0.1"},
				{"PublicIP", "203.0.113.1"},
				{"State", "running"},
				{"KeyName", "key-1"},
				{"AZ", "ap-northeast-1a"},
				{"LaunchTime", "2026-01-11"},
			},
		},
		{
			name:    "cfnParameterColumns / CfnParameter",
			columns: cfnParameterColumns,
			row: awsinternal.CfnParameter{
				Key:           "Env",
				Value:         "prod",
				ResolvedValue: "prod-resolved",
			}.ToRow(),
			want: []colValue{
				{"Key", "Env"},
				{"Value", "prod"},
				{"ResolvedValue", "prod-resolved"},
			},
		},
		{
			name:    "cfnOutputColumns / CfnOutput",
			columns: cfnOutputColumns,
			row: awsinternal.CfnOutput{
				Key:         "BucketArn",
				Value:       "arn:aws:s3:::bucket-1",
				ExportName:  "stack-1-bucket-arn",
				Description: "output description",
			}.ToRow(),
			want: []colValue{
				{"Key", "BucketArn"},
				{"Value", "arn:aws:s3:::bucket-1"},
				{"ExportName", "stack-1-bucket-arn"},
				{"Description", "output description"},
			},
		},
		{
			name:    "cfnTagColumns / CfnTag",
			columns: cfnTagColumns,
			row: awsinternal.CfnTag{
				Key:   "team",
				Value: "infra",
			}.ToRow(),
			want: []colValue{
				{"Key", "team"},
				{"Value", "infra"},
			},
		},
		{
			name:    "cfnChangeColumns / CfnChangeDetail",
			columns: cfnChangeColumns,
			row: awsinternal.CfnChangeDetail{
				Action:       "Modify",
				LogicalID:    "MyBucket",
				ResourceType: "AWS::S3::Bucket",
				Replacement:  "False",
			}.ToRow(),
			want: []colValue{
				{"Action", "Modify"},
				{"LogicalID", "MyBucket"},
				{"ResourceType", "AWS::S3::Bucket"},
				{"Replacement", "False"},
			},
		},
		{
			name:    "ecsServiceColumns / ECSServiceInfo",
			columns: ecsServiceColumns,
			row: awsinternal.ECSServiceInfo{
				ClusterName:    "cluster-1",
				ServiceName:    "svc-1",
				TaskDefinition: "td-1",
				Status:         "ACTIVE",
				DesiredCount:   1,
				RunningCount:   2,
				PendingCount:   3,
			}.ToRow(),
			want: []colValue{
				{"ClusterName", "cluster-1"},
				{"ServiceName", "svc-1"},
				{"TaskDefinition", "td-1"},
				{"Status", "ACTIVE"},
				{"DesiredTasks", "1"},
				{"RunningTasks", "2"},
				{"PendingTasks", "3"},
			},
		},
		{
			name:    "ecsTaskColumns / ECSTaskInfo",
			columns: ecsTaskColumns,
			row: awsinternal.ECSTaskInfo{
				TaskDefinition:  "td-1",
				TaskID:          "task-1",
				ContainerName:   "app",
				LastStatus:      "RUNNING",
				DesiredStatus:   "STOPPED",
				HealthStatus:    "HEALTHY",
				LaunchType:      "FARGATE",
				PlatformFamily:  "Linux",
				PlatformVersion: "1.4.0",
				StartedAt:       "2026-01-12",
			}.ToRow(),
			want: []colValue{
				{"TaskDefinition", "td-1"},
				{"Task", "task-1"},
				{"Container", "app"},
				{"LastStatus", "RUNNING"},
				{"DesiredStatus", "STOPPED"},
				{"HealthStatus", "HEALTHY"},
				{"LaunchType", "FARGATE"},
				{"PlatformFamily", "Linux"},
				{"PlatformVersion", "1.4.0"},
				{"StartedAt", "2026-01-12"},
			},
		},
		{
			name:    "ssmParamGetColumns / SSMParameterValue",
			columns: ssmParamGetColumns,
			row: awsinternal.SSMParameterValue{
				Name:    "/app/param",
				Type:    "SecureString",
				Value:   "secret-value",
				Version: 3,
				ARN:     "arn:aws:ssm:ap-northeast-1:111122223333:parameter/app/param",
			}.ToRow(),
			want: []colValue{
				{"Name", "/app/param"},
				{"Type", "SecureString"},
				{"Value", "secret-value"},
				{"Version", "3"},
				{"ARN", "arn:aws:ssm:ap-northeast-1:111122223333:parameter/app/param"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.want) != len(tt.columns) {
				t.Fatalf("test data broken: want has %d entries, columns has %d", len(tt.want), len(tt.columns))
			}
			if len(tt.row) != len(tt.columns) {
				t.Fatalf("ToRow() returned %d elements, want %d (same as columns)", len(tt.row), len(tt.columns))
			}
			for i := range tt.columns {
				if tt.columns[i].Header != tt.want[i].header {
					t.Errorf("columns[%d].Header = %q, want %q", i, tt.columns[i].Header, tt.want[i].header)
				}
				if tt.row[i] != tt.want[i].value {
					t.Errorf("row[%d] (%s) = %q, want %q", i, tt.columns[i].Header, tt.row[i], tt.want[i].value)
				}
			}
		})
	}
}
