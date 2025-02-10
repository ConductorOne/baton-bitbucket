package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	UsernameField = field.StringField("username", field.WithDescription("Username of administrator used to connect to the BitBucket API."),
		field.WithDisplayName("Username"), field.WithPlaceholder("Your Bitbucket username"), field.WithRequiredConnector(true))
	PasswordField = field.StringField("app-password", field.WithDescription("Application password used to connect to the BitBucket API."),
		field.WithIsSecret(true), field.WithDisplayName("Password"), field.WithPlaceholder("The Bitbucket app password for the username"), field.WithRequiredConnector(true))
	TokenField          = field.StringField("token", field.WithDescription("Access token (workspace or project scoped) used to connect to the BitBucket API."))
	ConsumerKeyField    = field.StringField("consumer-key", field.WithDescription("OAuth consumer key used to connect to the BitBucket API via oauth."))
	ConsumerSecretField = field.StringField("consumer-secret", field.WithDescription("The consumer secret used to connect to the BitBucket API via oauth."))
	WorkspacesField     = field.StringSliceField("workspaces", field.WithDescription("Limit syncing to specific workspaces by specifying workspace slugs."),
		field.WithDisplayName("Workspaces"), field.WithPlaceholder("List of Bitbucket workspaces to sync"), field.WithConnector(true))
)

var configFields = []field.SchemaField{
	UsernameField,
	PasswordField,
	TokenField,
	ConsumerKeyField,
	ConsumerSecretField,
	WorkspacesField,
}

var configRelations = []field.SchemaFieldRelationship{
	field.FieldsRequiredTogether(UsernameField, PasswordField),
	field.FieldsRequiredTogether(ConsumerKeyField, ConsumerSecretField),
}

//go:generate go run ./gen
var Config = field.Configuration{
	Fields:      configFields,
	Constraints: configRelations,
}
