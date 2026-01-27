package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	cfg "github.com/conductorone/baton-bitbucket/pkg/config"
	"github.com/conductorone/baton-bitbucket/pkg/connector"
	"github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/connectorrunner"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

var (
	version  = "dev"
	LoginURL = &url.URL{
		Scheme: "https",
		Host:   "bitbucket.org",
		Path:   "/site/oauth2/access_token",
	}
)

func main() {
	ctx := context.Background()

	_, cmd, err := config.DefineConfiguration(
		ctx,
		"baton-bitbucket",
		getConnector,
		cfg.Config,
		connectorrunner.WithDefaultCapabilitiesConnectorBuilder(&connector.Bitbucket{}),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	cmd.Version = version

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func constructAuth(c *cfg.Bitbucket) (uhttp.AuthCredentials, error) {
	if c.Token != "" {
		return uhttp.NewBearerAuth(c.Token), nil
	}

	if c.Username != "" {
		return uhttp.NewBasicAuth(c.Username, c.AppPassword), nil
	}

	if c.ConsumerKey != "" {
		return uhttp.NewOAuth2ClientCredentials(
			c.ConsumerKey,
			c.ConsumerSecret,
			LoginURL,
			nil,
		), nil
	}

	return nil, fmt.Errorf("invalid config")
}

func getConnector(ctx context.Context, c *cfg.Bitbucket) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)

	accessTokenNotSet := (c.Token == "")
	basicNotSet := (c.Username == "" || c.AppPassword == "")
	oauthNotSet := (c.ConsumerKey == "" || c.ConsumerSecret == "")

	if accessTokenNotSet && basicNotSet && oauthNotSet {
		return nil, fmt.Errorf("either an access token, username and password or consumer key and secret must be provided")
	}

	// compose the auth options
	auth, err := constructAuth(c)
	if err != nil {
		return nil, err
	}

	bitbucketConnector, err := connector.New(ctx, c.Workspaces, auth)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}

	conn, err := connectorbuilder.NewConnector(ctx, bitbucketConnector)
	if err != nil {
		l.Error("error creating connector", zap.Error(err))
		return nil, err
	}

	return conn, nil
}
