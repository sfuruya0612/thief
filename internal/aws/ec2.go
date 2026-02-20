// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Ec2Opts defines filtering options for EC2 instance operations.
type Ec2Opts struct {
	Running    bool   // If true, only return running instances
	InstanceId string // If specified, filter by this instance ID
}

// EC2Instance represents an EC2 instance for the interactive selection UI.
type EC2Instance struct {
	Name       string
	InstanceID string
}

// Title returns a formatted string representation of the EC2 instance for display.
func (i EC2Instance) Title() string {
	return fmt.Sprintf("%s (%s)", i.Name, i.InstanceID)
}

// ID returns the EC2 instance ID.
func (i EC2Instance) ID() string {
	return i.InstanceID
}

// EC2InstanceInfo holds the display fields for an EC2 instance.
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

// ec2Api defines the interface for EC2 API operations.
// This interface helps with testing by allowing mock implementations.
type ec2Api interface {
	DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeRegions(ctx context.Context, input *ec2.DescribeRegionsInput, opts ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
}

// NewEC2Client creates a new EC2 client using the specified AWS profile and region.
func NewEC2Client(profile, region string) (ec2Api, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		return nil, fmt.Errorf("create ec2 client: %w", err)
	}
	return ec2.NewFromConfig(cfg), nil
}

// getEc2TagValue returns the value of a specific EC2 tag from a list of tags.
// Returns an empty string if the tag is not found.
func getEc2TagValue(tags []types.Tag, key string) string {
	for _, tag := range tags {
		if *tag.Key == key {
			return *tag.Value
		}
	}
	return ""
}

// GenerateDescribeInstancesInput creates the input for the DescribeInstances API call
// with filters based on the provided options.
func GenerateDescribeInstancesInput(opts *Ec2Opts) (*ec2.DescribeInstancesInput, error) {
	i := &ec2.DescribeInstancesInput{}

	if opts.Running {
		i.Filters = append(i.Filters, types.Filter{
			Name:   aws.String("instance-state-name"),
			Values: []string{"running"},
		})
	}

	if opts.InstanceId != "" {
		i.Filters = append(i.Filters, types.Filter{
			Name:   aws.String("instance-id"),
			Values: []string{opts.InstanceId},
		})
	}

	return i, nil
}

// DescribeInstances calls the EC2 DescribeInstances API and returns the results
// as a typed slice of EC2InstanceInfo.
func DescribeInstances(api ec2Api, input *ec2.DescribeInstancesInput) ([]EC2InstanceInfo, error) {
	o, err := api.DescribeInstances(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var instances []EC2InstanceInfo
	for _, r := range o.Reservations {
		for _, i := range r.Instances {
			name := getEc2TagValue(i.Tags, "Name")

			lifecycle := "OnDemand"
			if i.InstanceLifecycle != "" {
				lifecycle = string(i.InstanceLifecycle)
			}

			privateIP := "None"
			if i.PrivateIpAddress != nil {
				privateIP = *i.PrivateIpAddress
			}

			publicIP := "None"
			if i.PublicIpAddress != nil {
				publicIP = *i.PublicIpAddress
			}

			keyName := "None"
			if i.KeyName != nil {
				keyName = *i.KeyName
			}

			instances = append(instances, EC2InstanceInfo{
				Name:         name,
				InstanceID:   aws.ToString(i.InstanceId),
				InstanceType: string(i.InstanceType),
				Lifecycle:    lifecycle,
				PrivateIP:    privateIP,
				PublicIP:     publicIP,
				State:        string(i.State.Name),
				KeyName:      keyName,
				AZ:           string(*i.Placement.AvailabilityZone),
				LaunchTime:   i.LaunchTime.String(),
			})
		}
	}

	return instances, nil
}

// GenerateDescribeRegionsInput creates the input for the DescribeRegions API call.
// Returns an empty input to fetch all available regions.
func GenerateDescribeRegionsInput() *ec2.DescribeRegionsInput {
	return &ec2.DescribeRegionsInput{}
}

// DescribeRegions calls the EC2 DescribeRegions API and returns a list of available region names.
func DescribeRegions(api ec2Api, input *ec2.DescribeRegionsInput) ([]string, error) {
	o, err := api.DescribeRegions(context.Background(), input)
	if err != nil {
		return nil, err
	}

	var regions []string
	for _, r := range o.Regions {
		regions = append(regions, *r.RegionName)
	}

	return regions, nil
}
