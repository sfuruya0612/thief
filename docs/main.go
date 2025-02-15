package main

import (
	"fmt"
	"os"

	"github.com/sfuruya0612/thief/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	fmt.Println("Generating documentation...")

	outputDir := "./docs"

	root := cmd.GetRootCmd()

	updateCommandDocs(root)

	err := doc.GenMarkdownTree(root, outputDir)
	if err != nil {
		fmt.Printf("Error generating documentation: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Documentation generated at %s\n", outputDir)
}

func updateCommandDocs(root *cobra.Command) {
	root.Example = `  # EC2インスタンスの一覧表示（タブ区切り）
  thief ec2
  thief ec2 ls

  # EC2インスタンスの一覧表示（CSV形式）
  thief ec2 --output csv
  thief ec2 -o csv

  # タグでフィルタリング
  thief ec2 --tag Environment:Production

  # RDSインスタンスの一覧表示
  thief rds
  thief rds ls`

	root.Long = `thief is a CLI tool that helps you to manage AWS resources.

Features:
- EC2インスタンスの一覧表示
  - インスタンス名、ID、タイプ、状態などの表示
  - タグによるフィルタリング
  - タブ区切りとCSV形式の出力に対応

- RDSインスタンスの一覧表示
  - インスタンス名、クラス、エンジン情報などの表示
  - タブ区切りとCSV形式の出力に対応`
}
