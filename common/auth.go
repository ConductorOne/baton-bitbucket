package common

import (
	"encoding/base64"
	"fmt"
)

const (
	BasicAuthScheme  = "Basic"
	BearerAuthScheme = "Bearer"
)

type AuthOption func() string

func WithBasicAuth(username, password string) AuthOption {
	return func() string {
		credentials := fmt.Sprintf("%s:%s", username, password)
		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))

		return fmt.Sprintf("%s %s", BasicAuthScheme, encodedCredentials)
	}
}

func WithBearerAuth(token string) AuthOption {
	return func() string {
		return fmt.Sprintf("%s %s", BearerAuthScheme, token)
	}
}
