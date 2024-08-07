package main

import (
	"github.com/conductorone/baton-sdk/pkg/field"
)

var (
	usernameField       = field.StringField("username", field.WithDescription("Username of administrator used to connect to the BitBucket API."))
	passwordField       = field.StringField("app-password", field.WithDescription("Application password used to connect to the BitBucket API."))
	tokenField          = field.StringField("token", field.WithDescription("Access token (workspace or project scoped) used to connect to the BitBucket API."))
	consumerKeyField    = field.StringField("consumer-key", field.WithDescription("OAuth consumer key used to connect to the BitBucket API via oauth."))
	consumerSecretField = field.StringField("consumer-secret", field.WithDescription("The consumer secret used to connect to the BitBucket API via oauth."))
	workspacesField     = field.StringSliceField("workspaces", field.WithDescription("Limit syncing to specific workspaces by specifying workspace slugs."))
)

var configFields = []field.SchemaField{
	usernameField,
	passwordField,
	tokenField,
	consumerKeyField,
	consumerSecretField,
	workspacesField,
}

var configRelations = []field.SchemaFieldRelationship{
	field.FieldsRequiredTogether(usernameField, passwordField),
	field.FieldsRequiredTogether(consumerKeyField, consumerSecretField),
}

var cfg = field.Configuration{
	Fields:      configFields,
	Constraints: configRelations,
}
