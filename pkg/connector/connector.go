package connector

import (
	"context"
	"fmt"
	"net/url"

	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	cfg "github.com/conductorone/baton-bitbucket/pkg/config"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
)

var (
	resourceTypeWorkspace = &v2.ResourceType{
		Id:          "workspace",
		DisplayName: "Workspace",
	}
	resourceTypeProject = &v2.ResourceType{
		Id:          "project",
		DisplayName: "Project",
		Traits: []v2.ResourceType_Trait{
			v2.ResourceType_TRAIT_GROUP,
		},
	}
	resourceTypeUserGroup = &v2.ResourceType{
		Id:          "user_group",
		DisplayName: "UserGroup",
		Traits: []v2.ResourceType_Trait{
			v2.ResourceType_TRAIT_GROUP,
		},
	}
	resourceTypeUser = &v2.ResourceType{
		Id:          "user",
		DisplayName: "User",
		Traits: []v2.ResourceType_Trait{
			v2.ResourceType_TRAIT_USER,
		},
	}
	resourceTypeRepository = &v2.ResourceType{
		Id:          "repository",
		DisplayName: "Repository",
	}

	loginURL = &url.URL{
		Scheme: "https",
		Host:   "bitbucket.org",
		Path:   "/site/oauth2/access_token",
	}
)

type Bitbucket struct {
	client     *bitbucket.Client
	workspaces []string
}

func (bb *Bitbucket) ResourceSyncers(_ context.Context) []connectorbuilder.ResourceSyncerV2 {
	return []connectorbuilder.ResourceSyncerV2{
		workspaceBuilder(bb.client, bb.workspaces),
		projectBuilder(bb.client),
		userBuilder(bb.client),
		userGroupBuilder(bb.client),
		repositoryBuilder(bb.client),
	}
}

// Metadata returns metadata about the connector.
func (bb *Bitbucket) Metadata(_ context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "Bitbucket",
	}, nil
}

// Validate hits the Bitbucket API to validate that the configured credentials are valid and compatible.
func (bb *Bitbucket) Validate(ctx context.Context) (annotations.Annotations, error) {
	user, err := bb.client.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("baton-bitbucket: failed to get current user: %w", err)
	}
	err = bb.setScope(user)
	if err != nil {
		return nil, err
	}

	if bb.client.IsUserScoped() {
		err = bb.client.SetWorkspaceIDs(ctx, bb.workspaces)
		if err != nil {
			return nil, fmt.Errorf("baton-bitbucket: failed to get workspace ids: %w", err)
		}
	}

	return nil, nil
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
			loginURL,
			nil,
		), nil
	}

	return nil, fmt.Errorf("invalid config")
}

// New is the connector constructor matching cli.NewConnector[*cfg.Bitbucket].
func New(ctx context.Context, c *cfg.Bitbucket, _ *cli.ConnectorOpts) (connectorbuilder.ConnectorBuilderV2, []connectorbuilder.Opt, error) {
	accessTokenNotSet := (c.Token == "")
	basicNotSet := (c.Username == "" || c.AppPassword == "")
	oauthNotSet := (c.ConsumerKey == "" || c.ConsumerSecret == "")

	if accessTokenNotSet && basicNotSet && oauthNotSet {
		return nil, nil, fmt.Errorf("either an access token, username and password or consumer key and secret must be provided")
	}

	auth, err := constructAuth(c)
	if err != nil {
		return nil, nil, err
	}

	httpClient, err := auth.GetClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("baton-bitbucket: failed to get http client: %w", err)
	}

	client, err := bitbucket.NewClient(ctx, httpClient)
	if err != nil {
		return nil, nil, err
	}

	return &Bitbucket{
		client:     client,
		workspaces: c.Workspaces,
	}, nil, nil
}

func (bb *Bitbucket) setScope(user *bitbucket.User) error {
	// check the type of user then set the scope
	switch user.Type {
	case "user":
		bb.client.SetupUserScope(user.Id)
	case "team":
		bb.client.SetupWorkspaceScope(user.Id)
	default:
		return fmt.Errorf("baton-bitbucket: unsupported user type: %s", user.Type)
	}
	return nil
}
