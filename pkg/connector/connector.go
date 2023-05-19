package connector

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ConductorOne/baton-bitbucket/common"
	"github.com/ConductorOne/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
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
	// get the scope of used credentials
	user, err := bb.client.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get current user: %w", err)
	}

	// check the type of user
	switch user.Type {
	case "user":
		bb.client.SetupUserScope(user.Id)
	case "team":
		bb.client.SetupWorkspaceScope(user.Id)
	default:
		return nil, fmt.Errorf("bitbucket-connector: unsupported user type: %s", user.Type)
	}

	// check if a user has the required permissions workspaces he belongs to
	if bb.client.IsUserScoped() {
		workspaceIds, err := bb.client.WorkspaceIds(ctx)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to get workspace ids: %w", err)
		}

		for _, workspaceId := range workspaceIds {
			err = bb.ValidateWorkspace(ctx, workspaceId)
			if err != nil {
				return nil, err
			}
		}
	} else {
		workspaceId, err := bb.client.WorkspaceId()
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to get workspace id: %w", err)
		}

		err = bb.ValidateWorkspace(ctx, workspaceId)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

/**
 * Since Bitbucket does not support listing specific permissions,
 * we need to validate it by checking if the user can list the resources for each resource type.
 *
 * If we can list the resources, we know that the user has the required permissions.
 * If we can't list the resources, we know that the user does not have the required permissions.
 */
func (bb *Bitbucket) ValidateWorkspace(ctx context.Context, workspaceId string) error {
	pagination := bitbucket.PaginationVars{
		Limit: 1,
	}

	// Check if we can list the projects
	projects, err := bb.client.GetAllWorkspaceProjects(ctx, workspaceId)
	if err != nil {
		return fmt.Errorf("bitbucket-connector: user is not able to list projects: %w", err)
	}

	// Check if we can list permissions for each project
	for _, project := range projects {
		_, _, err := bb.client.GetProjectGroupPermissions(ctx, workspaceId, project.Key, pagination)
		if err != nil {
			return fmt.Errorf("bitbucket-connector: user is not able to list project permissions: %w", err)
		}

		repositories, err := bb.client.GetAllProjectRepos(ctx, workspaceId, project.Id)
		if err != nil {
			return fmt.Errorf("bitbucket-connector: user is not able to list project repositories: %w", err)
		}

		// Check if we can list permissions for each repository
		for _, repository := range repositories {
			_, _, err := bb.client.GetRepositoryGroupPermissions(ctx, workspaceId, repository.Slug, pagination)
			if err != nil {
				return fmt.Errorf("bitbucket-connector: user is not able to list repository permissions: %w", err)
			}
		}
	}

	// Check if we can list user groups
	_, err = bb.client.GetWorkspaceUserGroups(ctx, workspaceId)
	if err != nil {
		return fmt.Errorf("bitbucket-connector: user is not able to list user groups: %w", err)
	}

	// Check if we can list users
	_, _, err = bb.client.GetWorkspaceMembers(ctx, workspaceId, pagination)
	if err != nil {
		return fmt.Errorf("bitbucket-connector: user is not able to list users: %w", err)
	}

	return nil
}

func New(ctx context.Context, workspaces []string, auth common.AuthOption) (*Bitbucket, error) {
	httpClient, err := uhttp.NewClient(ctx, uhttp.WithLogger(true, ctxzap.Extract(ctx)))
	if err != nil {
		return nil, err
	}

	auth, err = resolveAuth(auth, httpClient, ctx)
	if err != nil {
		return nil, err
	}

	return &Bitbucket{
		client:     bitbucket.NewClient(auth.Apply(), httpClient),
		workspaces: workspaces,
	}, nil
}

func resolveAuth(auth common.AuthOption, httpClient *http.Client, ctx context.Context) (common.AuthOption, error) {
	if oauth, ok := auth.(common.OAuth2Auth); ok {
		accessToken, err := bitbucket.Login(httpClient, ctx, oauth.Apply())
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to login: %w", err)
		}

		return common.BearerAuth{
			Token: accessToken,
		}, nil
	}

	return auth, nil
}
