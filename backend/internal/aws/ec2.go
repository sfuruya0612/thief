package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// EC2Resource represents a single EC2 instance.
type EC2Resource struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	State        string            `json:"state"`
	InstanceType string            `json:"instance_type"`
	AZ           string            `json:"az"`
	PrivateIP    string            `json:"private_ip"`
	PublicIP     string            `json:"public_ip"`
	VpcID        string            `json:"vpc_id"`
	Tags         map[string]string `json:"tags"`
	CostMonthly  float64           `json:"cost_monthly"`
	LaunchTime   time.Time         `json:"launch_time"`
}

func (r EC2Resource) ResourceID() string    { return r.ID }
func (r EC2Resource) ResourceName() string  { return r.Name }
func (r EC2Resource) ResourceState() string { return NormalizeState(r.State) }
func (r EC2Resource) ServiceName() string   { return "ec2" }

// ListEC2Resources returns all EC2 instances for the given profile/region.
func ListEC2Resources(ctx context.Context, profile, region string) ([]EC2Resource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ec2.Client {
		return ec2.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []EC2Resource
	paginator := ec2.NewDescribeInstancesPaginator(client, &ec2.DescribeInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe ec2 instances: %w", err)
		}
		for _, r := range page.Reservations {
			for _, inst := range r.Instances {
				if inst.State != nil && inst.State.Name == ec2types.InstanceStateNameTerminated {
					continue
				}
				resources = append(resources, ec2FromInstance(inst))
			}
		}
	}
	return resources, nil
}

func ec2FromInstance(inst ec2types.Instance) EC2Resource {
	tags := tagsToMap(inst.Tags)
	launch := time.Time{}
	if inst.LaunchTime != nil {
		launch = *inst.LaunchTime
	}
	r := EC2Resource{
		ID:           ptrStr(inst.InstanceId),
		Name:         tags["Name"],
		State:        DisplayState(string(inst.State.Name)),
		InstanceType: string(inst.InstanceType),
		AZ:           ptrStr(inst.Placement.AvailabilityZone),
		PrivateIP:    ptrStr(inst.PrivateIpAddress),
		PublicIP:     ptrStr(inst.PublicIpAddress),
		VpcID:        ptrStr(inst.VpcId),
		Tags:         tags,
		LaunchTime:   launch,
	}
	return r
}

// EC2ListOptions は CLI 向け EC2 一覧のフィルタ条件。
type EC2ListOptions struct {
	// Running が true のとき running 状態のインスタンスのみ返す。
	Running bool
	// InstanceIDs が非空のとき指定 ID のインスタンスのみ返す。
	InstanceIDs []string
}

// EC2InstanceInfo はレガシー CLI 互換の EC2 表示用フィールドを保持する。
type EC2InstanceInfo struct {
	Name         string
	InstanceID   string
	InstanceType string
	Lifecycle    string
	PrivateIP    string
	PublicIP     string
	State        string
	KeyName      string
	AZ           string
	LaunchTime   string
}

// ToRow converts EC2InstanceInfo to a string slice suitable for table formatting.
func (i EC2InstanceInfo) ToRow() []string {
	return []string{
		i.Name, i.InstanceID, i.InstanceType, i.Lifecycle,
		i.PrivateIP, i.PublicIP, i.State, i.KeyName, i.AZ, i.LaunchTime,
	}
}

// ListEC2Instances はレガシー CLI 互換のフィールドで EC2 インスタンス一覧を返す。
// ListEC2Resources と異なり terminated を除外せず、running / instance-id フィルタに対応する。
func ListEC2Instances(ctx context.Context, profile, region string, opts EC2ListOptions) ([]EC2InstanceInfo, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ec2.Client {
		return ec2.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	input := &ec2.DescribeInstancesInput{}
	if opts.Running {
		input.Filters = append(input.Filters, ec2types.Filter{
			Name:   aws.String("instance-state-name"),
			Values: []string{"running"},
		})
	}
	if len(opts.InstanceIDs) > 0 {
		input.Filters = append(input.Filters, ec2types.Filter{
			Name:   aws.String("instance-id"),
			Values: opts.InstanceIDs,
		})
	}

	var instances []EC2InstanceInfo
	paginator := ec2.NewDescribeInstancesPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe ec2 instances: %w", err)
		}
		for _, r := range page.Reservations {
			for _, inst := range r.Instances {
				instances = append(instances, ec2InstanceInfoFromSDK(inst))
			}
		}
	}
	return instances, nil
}

func ec2InstanceInfoFromSDK(inst ec2types.Instance) EC2InstanceInfo {
	lifecycle := "OnDemand"
	if inst.InstanceLifecycle != "" {
		lifecycle = string(inst.InstanceLifecycle)
	}

	privateIP := "None"
	if inst.PrivateIpAddress != nil {
		privateIP = *inst.PrivateIpAddress
	}

	publicIP := "None"
	if inst.PublicIpAddress != nil {
		publicIP = *inst.PublicIpAddress
	}

	keyName := "None"
	if inst.KeyName != nil {
		keyName = *inst.KeyName
	}

	state := ""
	if inst.State != nil {
		state = string(inst.State.Name)
	}

	az := ""
	if inst.Placement != nil {
		az = ptrStr(inst.Placement.AvailabilityZone)
	}

	launchTime := ""
	if inst.LaunchTime != nil {
		launchTime = inst.LaunchTime.String()
	}

	return EC2InstanceInfo{
		Name:         tagsToMap(inst.Tags)["Name"],
		InstanceID:   ptrStr(inst.InstanceId),
		InstanceType: string(inst.InstanceType),
		Lifecycle:    lifecycle,
		PrivateIP:    privateIP,
		PublicIP:     publicIP,
		State:        state,
		KeyName:      keyName,
		AZ:           az,
		LaunchTime:   launchTime,
	}
}

func tagsToMap(tags []ec2types.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[ptrStr(t.Key)] = ptrStr(t.Value)
	}
	return m
}

func tagMapStr(tags map[string]string) string {
	var parts []string
	for k, v := range tags {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrInt32(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

func ptrInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func ptrBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
