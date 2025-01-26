package cmd

import (
	"fmt"

	"github.com/sfuruya0612/thief/internal/util"
	"github.com/spf13/cobra"
)

var elasticacheCmd = &cobra.Command{
	Use:   "elasticache",
	Short: "Manage ElastiCache",
}

var elasticacheListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List ElastiCache clusters",
	Run:   listElastiCacheClusters,
}

var elasticacheColumns = []util.Column{
	{Header: "Name", Width: 65},
}

func listElastiCacheClusters(cmd *cobra.Command, args []string) {
	fmt.Println("List ElastiCache clusters")
}
