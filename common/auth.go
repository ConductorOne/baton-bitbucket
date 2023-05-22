package common

import (
	"encoding/base64"
	"fmt"
)

const (
	BasicAuthScheme  = "Basic"
	BearerAuthScheme = "Bearer"
)

type AuthOption interface {
	Apply() string
}

type BasicAuth struct {
	Username string
	Password string
}

func (auth BasicAuth) Apply() string {
	credentials := fmt.Sprintf("%s:%s", auth.Username, auth.Password)
	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))

	return fmt.Sprintf("%s %s", BasicAuthScheme, encodedCredentials)
}

type BearerAuth struct {
	Token string
}

func (auth BearerAuth) Apply() string {
	return fmt.Sprintf("%s %s", BearerAuthScheme, auth.Token)
}

type OAuth2Auth struct {
	ClientId     string
	ClientSecret string
}

func (auth OAuth2Auth) Apply() string {
	credentials := fmt.Sprintf("%s:%s", auth.ClientId, auth.ClientSecret)
	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))

	return fmt.Sprintf("%s %s", BasicAuthScheme, encodedCredentials)
}
