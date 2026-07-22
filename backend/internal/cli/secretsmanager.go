package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var secretListColumns = []util.Column{
	{Header: "Name"},
	{Header: "Description"},
	{Header: "LastChanged"},
}

func newSecretsManagerCmd() *cobra.Command {
	secretsCmd := &cobra.Command{
		Use:     "secretsmanager",
		Aliases: []string{"secrets"},
		Short:   "Secrets Manager commands",
	}

	lsCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List Secrets Manager secrets",
		Long:    "Retrieves and displays a list of Secrets Manager secrets (metadata only; secret values are not shown).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.SecretInfo]{
				Columns:  secretListColumns,
				EmptyMsg: "No secrets found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.SecretInfo, error) {
					return awsinternal.ListSecretInfos(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	putCmd := &cobra.Command{
		Use:   "put <name>",
		Short: "Update a Secrets Manager secret value",
		Long: "Stores a new value for an existing secret (PutSecretValue creates a new version). " +
			"Description, tags, and the encryption key are retained. " +
			"Provide the new value with --value, or omit it to read the value from stdin.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			value, err := readUpdateValue(cmd, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if err := awsinternal.PutSecretValue(context.Background(), cfg.Profile, cfg.Region, args[0], value); err != nil {
				return err
			}
			cmd.Printf("Updated secret %s\n", args[0])
			return nil
		},
	}
	putCmd.Flags().StringP("value", "", "", "New secret value (if omitted, read from stdin)")

	secretsCmd.AddCommand(lsCmd, putCmd)
	return secretsCmd
}
