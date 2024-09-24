package config

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	ConsumerKeyField = field.StringField(
		"consumer-key",
		field.WithDescription("OAuth consumer key used to connect to the BitBucket API via oauth."),
	)
	ConsumerSecretField = field.StringField(
		"consumer-secret",
		field.WithDescription("The consumer secret used to connect to the BitBucket API via oauth."),
	)
	PasswordField = field.StringField(
		"app-password",
		field.WithDescription("Application password used to connect to the BitBucket API."),
	)
	TokenField = field.StringField(
		"token",
		field.WithDescription("Access token (workspace or project scoped) used to connect to the BitBucket API."),
	)
	UsernameField = field.StringField(
		"username",
		field.WithDescription("Username of administrator used to connect to the BitBucket API."),
	)
	WorkspacesField = field.StringSliceField(
		"workspaces",
		field.WithDescription("Limit syncing to specific workspaces by specifying workspace slugs."),
	)
	configFields = []field.SchemaField{
		UsernameField,
		PasswordField,
		TokenField,
		ConsumerKeyField,
		ConsumerSecretField,
		WorkspacesField,
	}
	configRelations = []field.SchemaFieldRelationship{
		field.FieldsRequiredTogether(UsernameField, PasswordField),
		field.FieldsRequiredTogether(ConsumerKeyField, ConsumerSecretField),
	}
	ConfigurationSchema = field.Configuration{
		Fields:      configFields,
		Constraints: configRelations,
	}
)
