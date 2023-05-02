package main

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/spf13/cobra"
)

// config defines the external configuration required for the connector to run.
type config struct {
	cli.BaseConfig `mapstructure:",squash"` // Puts the base config options in the same place as the connector options

	Workspaces  []string `mapstructure:"workspaces"`
	AccessToken string   `mapstructure:"token"`
	Username    string   `mapstructure:"username"`
	Password    string   `mapstructure:"password"`
}

// validateConfig is run after the configuration is loaded, and should return an error if it isn't valid.
func validateConfig(ctx context.Context, cfg *config) error {
	if cfg.AccessToken == "" && (cfg.Username == "" || cfg.Password == "") {
		return fmt.Errorf("either an access token or a username and password must be provided")
	}

	return nil
}

// cmdFlags sets the cmdFlags required for the connector.
func cmdFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("token", "", "The BitBucket access token (workspace or project scoped) used to connect to the Github API. ($BATON_TOKEN)")
	cmd.PersistentFlags().String("username", "", "The BitBucket username of administrator used to connect to the Github API. ($BATON_USERNAME)")
	cmd.PersistentFlags().String("password", "", "The BitBucket application password used to connect to the Github API. ($BATON_PASSWORD)")
	cmd.PersistentFlags().StringSlice("workspaces", []string{}, "Limit syncing to specific workspaces. ($BATON_WORKSPACES)")
}
