package main

import (
	"context"

	cfg "github.com/conductorone/baton-bitbucket/pkg/config"
	"github.com/conductorone/baton-bitbucket/pkg/connector"
	"github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorrunner"
)

var version = "dev"

func main() {
	ctx := context.Background()
	config.RunConnector(
		ctx,
		"baton-bitbucket",
		version,
		cfg.Config,
		connector.New,
		connectorrunner.WithSessionStoreEnabled(),
		connectorrunner.WithDefaultCapabilitiesConnectorBuilderV2(&connector.Bitbucket{}),
	)
}
