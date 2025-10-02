package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	UsernameField = field.StringField(
		"username",
		field.WithDisplayName("Username"),
		field.WithDescription("Username of administrator used to connect to the Bitbucket API."),
		field.WithRequired(true),
	)
	PasswordField = field.StringField(
		"app-password",
		field.WithDisplayName("API Token/App Password"),
		field.WithDescription("The scoped API token or application password (deprecated) used to connect to the Bitbucket API."),
		field.WithIsSecret(true),
	)
	TokenField = field.StringField(
		"token",
		field.WithDisplayName("Access token"),
		field.WithDescription("Access token (workspace or project scoped) used to connect to the Bitbucket API."),
		field.WithIsSecret(true),
	)
	ConsumerKeyField = field.StringField(
		"consumer-key",
		field.WithDisplayName("Consumer key"),
		field.WithDescription("OAuth consumer key used to connect to the Bitbucket API via oauth."),
	)
	ConsumerSecretField = field.StringField(
		"consumer-secret",
		field.WithDisplayName("Consumer Secret"),
		field.WithDescription("The consumer secret used to connect to the Bitbucket API via oauth."),
		field.WithIsSecret(true),
	)
	WorkspacesField = field.StringSliceField(
		"workspaces",
		field.WithDisplayName("Workspaces"),
		field.WithDescription("Limit syncing to specific workspaces by specifying workspace slugs."),
	)
)

var ConfigurationFields = []field.SchemaField{
	UsernameField,
	PasswordField,
	TokenField,
	ConsumerKeyField,
	ConsumerSecretField,
	WorkspacesField,
}

var ConfigRelations = []field.SchemaFieldRelationship{
	field.FieldsRequiredTogether(ConsumerKeyField, ConsumerSecretField),
}

//go:generate go run ./gen
var Config = field.NewConfiguration(ConfigurationFields,
	field.WithConstraints(ConfigRelations...),
	field.WithConnectorDisplayName("Bitbucket"),
	field.WithHelpUrl("/docs/baton/bitbucket"),
	field.WithIconUrl("/static/app-icons/bitbucket.svg"),
)
