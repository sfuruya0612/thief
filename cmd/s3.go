package cmd

import (
	"fmt"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
)

func init() {
	// add s3 command
	rootCmd.AddCommand(s3Cmd)

	// add s3 list command
	s3Cmd.AddCommand(s3ListCmd)
}

// s3Cmd represents the s3 command
var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "S3 commands",
}

// s3ListCmd represents the s3 list command
var s3ListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List S3 buckets",
	Long:    "Retrieves and displays a list of S3 buckets in the AWS account.",
	RunE:    displayS3Buckets,
}

var s3Columns = []util.Column{
	{Header: "BucketName", Width: 50},
	{Header: "CreationDate", Width: 20},
}

// displayS3Buckets retrieves and displays S3 buckets.
func displayS3Buckets(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()

	client, err := aws.NewS3Client(profile, region)
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}

	input := aws.GenerateListBucketsInput()

	buckets, err := aws.ListBuckets(client, input)
	if err != nil {
		return fmt.Errorf("list S3 buckets: %w", err)
	}

	if len(buckets) == 0 {
		cmd.Println("No S3 buckets found")
		return nil
	}

	formatter := util.NewTableFormatter(s3Columns, cmd.Flag("output").Value.String())

	if cmd.Flag("no-header").Value.String() == "false" {
		formatter.PrintHeader()
	}

	formatter.PrintRows(buckets)
	return nil
}
