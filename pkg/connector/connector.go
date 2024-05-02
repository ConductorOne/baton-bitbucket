package connector

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"

	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
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

// Get all the valid workspaces, ie. the workspaces that the user has access to.
func (bb *Bitbucket) getValidWorkspaces(ctx context.Context) ([]string, error) {
	workspaceObject := workspaceBuilder(bb.client, bb.workspaces)
	workspaceResources, _, _, err := workspaceObject.List(ctx, nil, &pagination.Token{Token: ""})
	if err != nil {
		return nil, err
	}
	validWorkspaces := make([]string, 0)
	for _, workspace := range workspaceResources {
		ok, err := bb.checkPermissions(ctx, workspace)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to verify permissions: %w", err)
		}
		if !ok {
			continue
		}
		validWorkspaces = append(validWorkspaces, workspace.DisplayName)
	}
	return validWorkspaces, nil
}

func (bb *Bitbucket) checkPermissions(ctx context.Context, workspace *v2.Resource) (bool, error) {
	l := ctxzap.Extract(ctx)
	logMissingPermission := func(obj string) {
		l.Error(
			"missing permission to list object in workspace",
			zap.String("workspace", workspace.DisplayName),
			zap.String("workspace id", workspace.Id.Resource),
			zap.String("object", obj),
		)
	}
	paginationVars := bitbucket.PaginationVars{
		Limit: 1,
		Page:  "",
	}
	_, err := bb.client.GetWorkspaceUserGroups(ctx, workspace.Id.Resource)
	if err != nil {
		if isPermissionDeniedErr(err) {
			logMissingPermission("userGroups")
			return false, nil
		}
		return false, err
	}
	_, _, err = bb.client.GetWorkspaceMembers(ctx, workspace.Id.Resource, paginationVars)
	if err != nil {
		if isPermissionDeniedErr(err) {
			logMissingPermission("users")
			return false, nil
		}
		return false, err
	}
	_, _, err = bb.client.GetWorkspaceProjects(ctx, workspace.Id.Resource, paginationVars)
	if err != nil {
		if isPermissionDeniedErr(err) {
			logMissingPermission("projects")
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Validate hits the Bitbucket API to validate that the configured credentials are valid and compatible.
func (bb *Bitbucket) Validate(ctx context.Context) (annotations.Annotations, error) {
	// get the scope of used credentials
	user, err := bb.client.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get current user: %w", err)
	}
	err = bb.setScope(user)
	if err != nil {
		return nil, err
	}
	bb.workspaces, err = bb.getValidWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get valid workspaces: %w", err)
	}
	return nil, nil
}

func New(ctx context.Context, workspaces []string, auth uhttp.AuthCredentials) (*Bitbucket, error) {
	httpClient, err := auth.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get http client: %w", err)
	}

	return &Bitbucket{
		client:     bitbucket.NewClient(httpClient),
		workspaces: workspaces,
	}, nil
}

func (bb *Bitbucket) setScope(user *bitbucket.User) error {
	// check the type of user then set the scope
	switch user.Type {
	case "user":
		bb.client.SetupUserScope(user.Id)
	case "team":
		bb.client.SetupWorkspaceScope(user.Id)
	default:
		return fmt.Errorf("bitbucket-connector: unsupported user type: %s", user.Type)
	}
	return nil
}
