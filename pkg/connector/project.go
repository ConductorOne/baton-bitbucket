package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	grant "github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

const repoEntitlement = "repository"
const (
	roleRead   = "read"
	roleWrite  = "write"
	roleCreate = "create-repo"
	roleAdmin  = "admin"
	roleNone   = "none"
)

var projectPermissions = []string{roleRead, roleWrite, roleCreate, roleAdmin}

type projectResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (p *projectResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return p.resourceType
}

func ComposeProjectId(workspaceId string, projectId string, key string) string {
	return fmt.Sprintf("%s:%s:%s", workspaceId, projectId, key)
}

func DecomposeProjectId(id string) (string, string, string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("bitbucket-connector: invalid project resource id")
	}

	// We need to split the project id into workspace and project id
	return parts[0], parts[1], parts[2], nil
}

// Create a new connector resource for an Bitbucket Project.
func projectResource(ctx context.Context, project *bitbucket.Project, parentResourceID *v2.ResourceId) (*v2.Resource, error) {
	profile := map[string]interface{}{
		"project_id":   project.Id,
		"project_name": project.Name,
		"project_key":  project.Key,
	}

	resource, err := rs.NewGroupResource(
		project.Name,
		resourceTypeProject,
		ComposeProjectId(parentResourceID.Resource, project.Id, project.Key),
		[]rs.GroupTraitOption{
			rs.WithGroupProfile(profile),
		},
		rs.WithParentResourceID(parentResourceID),
		rs.WithAnnotation(
			&v2.ChildResourceType{ResourceTypeId: resourceTypeRepository.Id},
		),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

func (p *projectResourceType) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	if parentId == nil {
		return nil, "", nil, nil
	}

	// parse the token
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeProject.Id})
	if err != nil {
		return nil, "", nil, err
	}

	projects, nextToken, err := p.client.GetWorkspaceProjects(
		ctx,
		parentId.Resource,
		bitbucket.PaginationVars{
			Limit: ResourcesPageSize,
			Page:  bag.PageToken(),
		},
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list projects: %w", err)
	}

	pageToken, err := bag.NextToken(nextToken)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource
	for _, project := range projects {
		projectCopy := project

		pr, err := projectResource(ctx, &projectCopy, parentId)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, pr)
	}

	return rv, pageToken, nil, nil
}

func (p *projectResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement
	assignmentOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(resourceTypeRepository),
		ent.WithDisplayName(fmt.Sprintf("%s Project %s", resource.DisplayName, repoEntitlement)),
		ent.WithDescription(fmt.Sprintf("Access to %s project in Bitbucket", resource.DisplayName)),
	}

	// create membership entitlement
	rv = append(rv, ent.NewAssignmentEntitlement(
		resource,
		repoEntitlement,
		assignmentOptions...,
	))

	// create entitlements for each project role (read, write, create, admin)
	for _, permission := range projectPermissions {
		permissionOptions := []ent.EntitlementOption{
			ent.WithGrantableTo(resourceTypeUser, resourceTypeUserGroup),
			ent.WithDisplayName(fmt.Sprintf("%s Project %s", resource.DisplayName, permission)),
			ent.WithDescription(fmt.Sprintf("%s access to %s project in Bitbucket", titleCase(permission), resource.DisplayName)),
		}

		rv = append(rv, ent.NewPermissionEntitlement(
			resource,
			permission,
			permissionOptions...,
		))
	}

	return rv, "", nil, nil
}

func (p *projectResourceType) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	bag, err := parsePageToken(token.Token, resource.Id)
	if err != nil {
		return nil, "", nil, err
	}

	workspaceId, projectId, projectKey, err := DecomposeProjectId(resource.Id.Resource)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Grant

	switch bag.ResourceTypeID() {
	case resourceTypeProject.Id:
		bag.Pop()
		bag.Push(pagination.PageState{
			ResourceTypeID: resourceTypeRepository.Id,
		})
		bag.Push(pagination.PageState{
			ResourceTypeID: resourceTypeUserGroup.Id,
		})
		bag.Push(pagination.PageState{
			ResourceTypeID: resourceTypeUser.Id,
		})

	// create a membership grant for each repository in the project
	case resourceTypeRepository.Id:
		repos, nextToken, err := p.client.GetProjectRepos(
			ctx,
			workspaceId,
			projectId,
			bitbucket.PaginationVars{
				Limit: ResourcesPageSize,
				Page:  bag.PageToken(),
			},
		)
		if err != nil {
			return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list project repositories: %w", err)
		}

		err = bag.Next(nextToken)
		if err != nil {
			return nil, "", nil, err
		}

		for _, repo := range repos {
			repoCopy := repo
			rr, err := repositoryResource(ctx, &repoCopy, &v2.ResourceId{Resource: resource.Id.Resource})
			if err != nil {
				return nil, "", nil, err
			}

			rv = append(
				rv,
				grant.NewGrant(
					resource,
					repoEntitlement,
					rr.Id,
				),
			)
		}

	// create a permission grant for each usergroup in the project
	case resourceTypeUserGroup.Id:
		permissions, nextToken, err := p.client.GetProjectGroupPermissions(
			ctx,
			workspaceId,
			projectKey,
			bitbucket.PaginationVars{
				Limit: ResourcesPageSize,
				Page:  bag.PageToken(),
			},
		)
		if err != nil {
			return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list project group permissions: %w", err)
		}

		err = bag.Next(nextToken)
		if err != nil {
			return nil, "", nil, err
		}

		for _, permission := range permissions {
			// check if the permission is supported project role
			if !contains(permission.Value, projectPermissions) {
				continue
			}

			groupCopy := permission.Group

			gr, err := userGroupResource(ctx, &groupCopy, &v2.ResourceId{Resource: workspaceId})
			if err != nil {
				return nil, "", nil, err
			}

			rv = append(
				rv,
				grant.NewGrant(
					resource,
					permission.Value,
					gr.Id,
				),
			)
		}

	// create a permission grant for each user in the project
	case resourceTypeUser.Id:
		permissions, nextToken, err := p.client.GetProjectUserPermissions(
			ctx,
			workspaceId,
			projectKey,
			bitbucket.PaginationVars{
				Limit: ResourcesPageSize,
				Page:  bag.PageToken(),
			},
		)
		if err != nil {
			return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list project user permissions: %w", err)
		}

		err = bag.Next(nextToken)
		if err != nil {
			return nil, "", nil, err
		}

		for _, permission := range permissions {
			// check if the permission is supported project role
			if !contains(permission.Value, projectPermissions) {
				continue
			}

			userCopy := permission.User

			ur, err := userResource(ctx, &userCopy, &v2.ResourceId{Resource: workspaceId})
			if err != nil {
				return nil, "", nil, err
			}

			rv = append(
				rv,
				grant.NewGrant(
					resource,
					permission.Value,
					ur.Id,
				),
			)
		}

	default:
		return nil, "", nil, fmt.Errorf("bitbucket-connector: invalid grant resource type: %s", bag.ResourceTypeID())
	}

	pageToken, err := bag.Marshal()
	if err != nil {
		return nil, "", nil, err
	}

	return rv, pageToken, nil, nil
}

func (p *projectResourceType) GetPermission(ctx context.Context, principal *v2.Resource, workspaceId, projectKey string) (*bitbucket.Permission, error) {
	if principal.Id.ResourceType == resourceTypeUser.Id {
		userPermission, err := p.client.GetProjectUserPermission(
			ctx,
			workspaceId,
			projectKey,
			principal.Id.Resource,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to get project user permission: %w", err)
		}

		return &userPermission.Permission, nil
	} else if principal.Id.ResourceType == resourceTypeUserGroup.Id {
		groupPermission, err := p.client.GetProjectGroupPermission(
			ctx,
			workspaceId,
			projectKey,
			principal.Id.Resource,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to get project group permission: %w", err)
		}

		return &groupPermission.Permission, nil
	}

	return nil, fmt.Errorf("bitbucket-connector: invalid principal resource type: %s", principal.Id.ResourceType)
}

func (p *projectResourceType) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principalIsUser := principal.Id.ResourceType == resourceTypeUser.Id
	principalIsGroup := principal.Id.ResourceType == resourceTypeUserGroup.Id

	if !principalIsUser && !principalIsGroup {
		l.Warn(
			"bitbucket-connector: only users and groups can be granted project permissions",
			zap.String("principal_id", principal.Id.Resource),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: only users and groups can be granted project permissions")
	}

	projectResourceId, slug, err := ParseEntitlementID(entitlement.Id)
	if err != nil {
		return nil, err
	}

	workspaceId, _, projectKey, err := DecomposeProjectId(projectResourceId.Resource)
	if err != nil {
		return nil, err
	}

	// check if the entitlement is for repository permission
	if slug == repoEntitlement {
		l.Warn(
			"bitbucket-connector: granting repository memberships is not supported",
			zap.String("entitlement_id", entitlement.Id),
		)

		return nil, fmt.Errorf("bitbucket-connector: granting repository memberships is not supported")
	}

	// check if the permission is supported project role
	if !contains(slug, projectPermissions) {
		return nil, fmt.Errorf("bitbucket-connector: unsupported project role: %s", slug)
	}

	permission, err := p.GetPermission(ctx, principal, workspaceId, projectKey)
	if err != nil {
		return nil, err
	}

	// warn if the principal already has a project permission
	if permission.Value != roleNone {
		l.Warn(
			"bitbucket-connector: principal already has a project permission",
		)
	}

	// update the project permission
	if principalIsUser {
		err = p.client.UpdateProjectUserPermission(
			ctx,
			workspaceId,
			projectKey,
			principal.Id.Resource,
			slug,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to update project user permission: %w", err)
		}
	} else if principalIsGroup {
		err = p.client.UpdateProjectGroupPermission(
			ctx,
			workspaceId,
			projectKey,
			principal.Id.Resource,
			slug,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to update project group permission: %w", err)
		}
	}

	return nil, nil
}

func (p *projectResourceType) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principal := grant.Principal
	entitlement := grant.Entitlement
	principalIsUser := principal.Id.ResourceType == resourceTypeUser.Id
	principalIsGroup := principal.Id.ResourceType == resourceTypeUserGroup.Id

	if !principalIsUser && !principalIsGroup {
		l.Warn(
			"bitbucket-connector: only users and groups can have project permissions revoked",
			zap.String("principal_id", principal.Id.Resource),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: only users and groups can have project permissions revoked")
	}

	projectResourceId, slug, err := ParseEntitlementID(entitlement.Id)
	if err != nil {
		return nil, err
	}

	workspaceId, _, projectKey, err := DecomposeProjectId(projectResourceId.Resource)
	if err != nil {
		return nil, err
	}

	if slug == repoEntitlement {
		l.Warn(
			"bitbucket-connector: revoking repository memberships is not supported",
			zap.String("entitlement_id", entitlement.Id),
		)

		return nil, fmt.Errorf("bitbucket-connector: revoking repository memberships is not supported")
	}

	permission, err := p.GetPermission(ctx, principal, workspaceId, projectKey)
	if err != nil {
		return nil, err
	}

	// check if the permission is supported project role
	if !contains(slug, projectPermissions) {
		return nil, fmt.Errorf("bitbucket-connector: unsupported project role: %s", permission.Value)
	}

	// warn if the principal already doesnt have this project permission
	if permission.Value == roleNone {
		l.Warn(
			"bitbucket-connector: principal already doesnt have this project permission",
		)
	}

	// remove the project permission
	if principalIsUser {
		err = p.client.DeleteProjectUserPermission(
			ctx,
			workspaceId,
			projectKey,
			principal.Id.Resource,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to remove project user permission: %w", err)
		}
	} else if principalIsGroup {
		err = p.client.DeleteProjectGroupPermission(
			ctx,
			workspaceId,
			projectKey,
			principal.Id.Resource,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to remove project group permission: %w", err)
		}
	}

	return nil, nil
}

func projectBuilder(client *bitbucket.Client) *projectResourceType {
	return &projectResourceType{
		resourceType: resourceTypeProject,
		client:       client,
	}
}
