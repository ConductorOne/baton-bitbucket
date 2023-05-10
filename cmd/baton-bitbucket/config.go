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

	Workspaces   []string `mapstructure:"workspaces"`
	AccessToken  string   `mapstructure:"token"`
	Username     string   `mapstructure:"username"`
	Password     string   `mapstructure:"password"`
	ClientId     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
}

// validateConfig is run after the configuration is loaded, and should return an error if it isn't valid.
func validateConfig(ctx context.Context, cfg *config) error {
	accessTokenNotSet := (cfg.AccessToken == "")
	basicNotSet := (cfg.Username == "" || cfg.Password == "")
	oauthNotSet := (cfg.ClientId == "" || cfg.ClientSecret == "")

	if accessTokenNotSet && basicNotSet && oauthNotSet {
		return fmt.Errorf("either an access token, username and password or client id and secret must be provided")
	}

	return nil
}

// cmdFlags sets the cmdFlags required for the connector.
func cmdFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("token", "", "Access token (workspace or project scoped) used to connect to the BitBucket API. ($BATON_TOKEN)")
	cmd.PersistentFlags().String("username", "", "Username of administrator used to connect to the BitBucket API. ($BATON_USERNAME)")
	cmd.PersistentFlags().String("password", "", "Application password used to connect to the BitBucket API. ($BATON_PASSWORD)")
	cmd.PersistentFlags().String("client_id", "", "Client id of an application used to connect to the BitBucket API via oauth. ($BATON_CLIENT_ID)")
	cmd.PersistentFlags().String("client_secret", "", "The client secret used to connect to the BitBucket API via oauth. ($BATON_CLIENT_SECRET)")
	cmd.PersistentFlags().StringSlice("workspaces", []string{}, "Limit syncing to specific workspaces. ($BATON_WORKSPACES)")
}
