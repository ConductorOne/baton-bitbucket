package common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	BearerAuthScheme = "Bearer"

	LoginBaseURL = "https://bitbucket.org/site/oauth2/access_token"
)

type AuthOption interface {
	Apply(req *http.Request)
	GetClient(ctx context.Context) (*http.Client, error)
}

type BasicAuth struct {
	Username string
	Password string
}

func (auth BasicAuth) Apply(req *http.Request) {
	req.SetBasicAuth(auth.Username, auth.Password)
}

func (auth BasicAuth) GetClient(ctx context.Context) (*http.Client, error) {
	httpClient, err := uhttp.NewClient(ctx, uhttp.WithLogger(true, nil))
	if err != nil {
		return nil, err
	}

	return httpClient, nil
}

type BearerAuth struct {
	Token string
}

func (auth BearerAuth) Apply(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", BearerAuthScheme, auth.Token))
}

func (auth BearerAuth) GetClient(ctx context.Context) (*http.Client, error) {
	httpClient, err := uhttp.NewClient(ctx, uhttp.WithLogger(true, nil))
	if err != nil {
		return nil, err
	}

	return httpClient, nil
}

type OAuth2Auth struct {
	cfg *clientcredentials.Config
}

func NewOAuth2Auth(clientId, clientSecret string) *OAuth2Auth {
	return &OAuth2Auth{
		cfg: &clientcredentials.Config{
			ClientID:     clientId,
			ClientSecret: clientSecret,
			TokenURL:     LoginBaseURL,
		},
	}
}

func (auth *OAuth2Auth) GetClient(ctx context.Context) (*http.Client, error) {
	ts := auth.cfg.TokenSource(ctx)
	httpClient := oauth2.NewClient(ctx, ts)

	return httpClient, nil
}

func (auth OAuth2Auth) Apply(req *http.Request) {
	// No need to set the Authorization header here, the oauth2 client does it automatically
}
