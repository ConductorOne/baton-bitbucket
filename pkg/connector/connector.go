package connector

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
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
)

type Bitbucket struct {
	client     *bitbucket.Client
	workspaces []string
}

func (bb *Bitbucket) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	return []connectorbuilder.ResourceSyncer{
		workspaceBuilder(bb.client, bb.workspaces),
		projectBuilder(bb.client),
		userBuilder(bb.client),
		userGroupBuilder(bb.client),
		repositoryBuilder(bb.client),
	}
}

// Metadata returns metadata about the connector.
func (bb *Bitbucket) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "Bitbucket",
	}, nil
}

// Validate hits the Bitbucket API to validate that the configured credentials are valid and compatible.
func (bb *Bitbucket) Validate(ctx context.Context) (annotations.Annotations, error) {
	// Bitbucket Cloud teams are deprecated (CHANGE-2770). All authentication
	// is now user-scoped, so we set user scope directly and discover
	// workspaces via the non-deprecated workspace permissions endpoint.
	bb.client.SetupUserScope("")

	err := bb.client.SetWorkspaceIDs(ctx, bb.workspaces)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get workspace ids: %w", err)
	}

	return nil, nil
}

func New(ctx context.Context, workspaces []string, auth uhttp.AuthCredentials) (*Bitbucket, error) {
	httpClient, err := auth.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get http client: %w", err)
	}

	client, err := bitbucket.NewClient(ctx, httpClient)
	if err != nil {
		return nil, err
	}
	return &Bitbucket{
		client:     client,
		workspaces: workspaces,
	}, nil
}

