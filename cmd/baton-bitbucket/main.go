package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/conductorone/baton-bitbucket/pkg/config"
	"github.com/conductorone/baton-bitbucket/pkg/connector"
	configschema "github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	version       = "dev"
	connectorName = "baton-bitbucket"
	LoginURL      = &url.URL{
		Scheme: "https",
		Host:   "bitbucket.org",
		Path:   "/site/oauth2/access_token",
	}
)

func main() {
	ctx := context.Background()

	_, cmd, err := configschema.DefineConfiguration(
		ctx,
		connectorName,
		getConnector,
		config.ConfigurationSchema,
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

func constructAuth(v *viper.Viper) (uhttp.AuthCredentials, error) {
	accessToken := v.GetString(config.TokenField.FieldName)
	username := v.GetString(config.UsernameField.FieldName)
	password := v.GetString(config.PasswordField.FieldName)
	consumerId := v.GetString(config.ConsumerKeyField.FieldName)
	consumerSecret := v.GetString(config.ConsumerSecretField.FieldName)

	if accessToken != "" {
		return uhttp.NewBearerAuth(accessToken), nil
	}

	if username != "" {
		return uhttp.NewBasicAuth(username, password), nil
	}

	if consumerId != "" {
		return uhttp.NewOAuth2ClientCredentials(
			consumerId,
			consumerSecret,
			LoginURL,
			nil,
		), nil
	}

	return nil, fmt.Errorf("invalid config")
}

func getConnector(ctx context.Context, v *viper.Viper) (types.ConnectorServer, error) {
	l := ctxzap.Extract(ctx)

	accessToken := v.GetString(config.TokenField.FieldName)
	accessTokenNotSet := (accessToken == "")
	username := v.GetString(config.UsernameField.FieldName)
	password := v.GetString(config.PasswordField.FieldName)
	consumerId := v.GetString(config.ConsumerKeyField.FieldName)
	consumerSecret := v.GetString(config.ConsumerSecretField.FieldName)

	basicNotSet := username == "" || password == ""
	oauthNotSet := consumerId == "" || consumerSecret == ""

	if accessTokenNotSet && basicNotSet && oauthNotSet {
		return nil, fmt.Errorf("either an access token, username and password or consumer key and secret must be provided")
	}

	// compose the auth options
	auth, err := constructAuth(v)
	if err != nil {
		return nil, err
	}

	bitbucketConnector, err := connector.New(
		ctx,
		v.GetStringSlice(config.WorkspacesField.FieldName),
		auth,
	)
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
