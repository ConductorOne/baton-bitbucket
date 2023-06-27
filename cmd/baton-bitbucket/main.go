package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ConductorOne/baton-bitbucket/common"
	"github.com/ConductorOne/baton-bitbucket/pkg/connector"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/sdk"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

var version = "dev"

func main() {
	ctx := context.Background()

	cfg := &config{}
	cmd, err := cli.NewCmd(ctx, "baton-bitbucket", cfg, validateConfig, getConnector, run)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	cmd.Version = version
	cmdFlags(cmd)

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func constructAuth(cfg *config) (common.AuthOption, error) {
	if cfg.AccessToken != "" {
		return common.BearerAuth{
			Token: cfg.AccessToken,
		}, nil
	}

	if cfg.Username != "" {
		return common.BasicAuth{
			Username: cfg.Username,
			Password: cfg.Password,
		}, nil
	}

	if cfg.ConsumerId != "" {
		return common.OAuth2Auth{
			ClientId:     cfg.ConsumerId,
			ClientSecret: cfg.ConsumerSecret,
		}, nil
	}

	return nil, fmt.Errorf("invalid config")
}

func getConnector(ctx context.Context, cfg *config) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)

	// compose the auth options
	auth, err := constructAuth(cfg)
	if err != nil {
		return nil, err
	}

	bitbucketConnector, err := connector.New(ctx, cfg.Workspaces, auth)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}

	c, err := connectorbuilder.NewConnector(ctx, bitbucketConnector)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}

	return c, nil
}

// run is where the process of syncing with the connector is implemented.
func run(ctx context.Context, cfg *config) error {
	l := ctxzap.Extract(ctx)

	c, err := getConnector(ctx, cfg)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return err
	}

	r, err := sdk.NewConnectorRunner(ctx, c, cfg.C1zPath)
	if err != nil {
		l.Error("error creating connector runner", zap.Error(err))
		return err
	}
	defer r.Close()

	err = r.Run(ctx)
	if err != nil {
		l.Error("error running connector", zap.Error(err))
		return err
	}

	return nil
}
