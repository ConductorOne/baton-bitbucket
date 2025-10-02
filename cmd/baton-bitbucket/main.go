package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/conductorone/baton-bitbucket/pkg/config"
	"github.com/conductorone/baton-bitbucket/pkg/connector"
	configSdk "github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

var (
	connectorName = "baton-bitbucket"
	version       = "dev"
	LoginURL      = &url.URL{
		Scheme: "https",
		Host:   "bitbucket.org",
		Path:   "/site/oauth2/access_token",
	}
)

func main() {
	ctx := context.Background()

	_, cmd, err := configSdk.DefineConfiguration(
		ctx,
		connectorName,
		getConnector,
		config.Config,
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

func constructAuth(cfg *config.Bitbucket) (uhttp.AuthCredentials, error) {
	accessToken := cfg.Token
	username := cfg.Username
	password := cfg.AppPassword
	consumerID := cfg.ConsumerKey
	consumerSecret := cfg.ConsumerSecret

	if accessToken != "" {
		return uhttp.NewBearerAuth(accessToken), nil
	}

	if username != "" {
		return uhttp.NewBasicAuth(username, password), nil
	}

	if consumerID != "" {
		return uhttp.NewOAuth2ClientCredentials(
			consumerID,
			consumerSecret,
			LoginURL,
			nil,
		), nil
	}

	return nil, fmt.Errorf("invalid config")
}

func getConnector(ctx context.Context, cfg *config.Bitbucket) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)

	accessToken := cfg.Token
	accessTokenNotSet := (accessToken == "")
	username := cfg.Username
	password := cfg.AppPassword
	consumerID := cfg.ConsumerKey
	consumerSecret := cfg.ConsumerSecret
	workspaces := cfg.Workspaces

	basicNotSet := (username == "" || password == "")
	oauthNotSet := (consumerID == "" || consumerSecret == "")

	if accessTokenNotSet && basicNotSet && oauthNotSet {
		return nil, fmt.Errorf("either an access token, username and password or consumer key and secret must be provided")
	}

	// compose the auth options
	auth, err := constructAuth(cfg)
	if err != nil {
		return nil, err
	}

	bitbucketConnector, err := connector.New(ctx, workspaces, auth)
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
