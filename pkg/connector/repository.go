package connector

import (
	"context"
	"errors"
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

var repositoryRoles = []string{roleRead, roleWrite, roleAdmin}

type repositoryResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (r *repositoryResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return r.resourceType
}

func ComposeRepositoryId(projectId, repositoryId string) string {
	return fmt.Sprintf("%s:%s", projectId, repositoryId)
}

func DecomposeRepositoryId(repositoryId string) (string, string, error) {
	parts := strings.Split(repositoryId, ":")
	// There needs to be at least 2 parts (project id and repository id)
	if len(parts) < 3 {
		return "", "", errors.New("bitbucket-connector: invalid repository resource id")
	}

	// Check if the project id is valid
	projectId := strings.Join(parts[0:len(parts)-1], ":")
	if _, _, _, err := DecomposeProjectId(projectId); err != nil {
		return "", "", errors.New("bitbucket-connector: invalid repository resource id, composed project id is invalid")
	}

	// Return the project id and repository id
	return projectId, parts[len(parts)-1], nil
}

// Create a new connector resource for an Bitbucket Repository.
func repositoryResource(ctx context.Context, repository *bitbucket.Repository, parentResourceID *v2.ResourceId) (*v2.Resource, error) {
	profile := map[string]interface{}{
		"repository_id":        repository.Id,
		"repository_name":      repository.Name,
		"repository_full_name": repository.FullName,
	}

	resource, err := rs.NewGroupResource(
		repository.FullName,
		resourceTypeRepository,
		ComposeRepositoryId(parentResourceID.Resource, repository.Id),
		[]rs.GroupTraitOption{
			rs.WithGroupProfile(profile),
		},
		rs.WithParentResourceID(parentResourceID),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

func (r *repositoryResourceType) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	if parentId == nil {
		return nil, "", nil, nil
	}

	// parse the token
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeRepository.Id})
	if err != nil {
		return nil, "", nil, err
	}

	workspaceId, projectId, _, err := DecomposeProjectId(parentId.Resource)
	if err != nil {
		return nil, "", nil, err
	}

	repositories, nextToken, err := r.client.GetProjectRepos(
		ctx,
		workspaceId,
		projectId,
		bitbucket.PaginationVars{
			Limit: ResourcesPageSize,
			Page:  bag.PageToken(),
		},
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list repositories: %w", err)
	}

	pageToken, err := bag.NextToken(nextToken)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource
	for _, repository := range repositories {
		repositoryCopy := repository

		tResource, err := repositoryResource(ctx, &repositoryCopy, parentId)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, tResource)
	}

	return rv, pageToken, nil, nil
}

func (r *repositoryResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement

	// create entitlements for each repository role (read, write, admin)
	for _, role := range repositoryRoles {
		permissionOptions := []ent.EntitlementOption{
			ent.WithGrantableTo(resourceTypeUser, resourceTypeUserGroup),
			ent.WithDisplayName(fmt.Sprintf("%s Repository %s", resource.DisplayName, role)),
			ent.WithDescription(fmt.Sprintf("%s access to %s repository in Bitbucket", titleCase(role), resource.DisplayName)),
		}

		rv = append(rv, ent.NewPermissionEntitlement(
			resource,
			role,
			permissionOptions...,
		))
	}

	return rv, "", nil, nil
}

func (r *repositoryResourceType) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	bag, err := parsePageToken(token.Token, resource.Id)
	if err != nil {
		return nil, "", nil, err
	}

	composedProjectId, repositoryId, err := DecomposeRepositoryId(resource.Id.Resource)
	if err != nil {
		return nil, "", nil, err
	}

	workspaceId, _, _, err := DecomposeProjectId(composedProjectId)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Grant
	switch bag.ResourceTypeID() {
	case resourceTypeRepository.Id:
		bag.Pop()
		bag.Push(pagination.PageState{
			ResourceTypeID: resourceTypeUserGroup.Id,
		})
		bag.Push(pagination.PageState{
			ResourceTypeID: resourceTypeUser.Id,
		})

	// create a permission grant for each usergroup in the repository
	case resourceTypeUserGroup.Id:
		permissions, nextToken, err := r.client.GetRepositoryGroupPermissions(
			ctx,
			workspaceId,
			repositoryId,
			bitbucket.PaginationVars{
				Limit: ResourcesPageSize,
				Page:  bag.PageToken(),
			},
		)
		if err != nil {
			return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list repository group permissions: %w", err)
		}

		err = bag.Next(nextToken)
		if err != nil {
			return nil, "", nil, err
		}

		for _, permission := range permissions {
			// check if the permission is supported repository role
			if !contains(permission.Value, repositoryRoles) {
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

	// create a permission grant for each user in the repository
	case resourceTypeUser.Id:
		permissions, nextToken, err := r.client.GetRepositoryUserPermissions(
			ctx,
			workspaceId,
			repositoryId,
			bitbucket.PaginationVars{
				Limit: ResourcesPageSize,
				Page:  bag.PageToken(),
			},
		)
		if err != nil {
			return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list repository user permissions: %w", err)
		}

		err = bag.Next(nextToken)
		if err != nil {
			return nil, "", nil, err
		}

		for _, permission := range permissions {
			// check if the permission is supported repository role
			if !contains(permission.Value, repositoryRoles) {
				continue
			}

			memberCopy := permission.User

			ur, err := userResource(ctx, &memberCopy, &v2.ResourceId{Resource: workspaceId})
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

func (r *repositoryResourceType) GetPermission(ctx context.Context, principal *v2.Resource, workspaceId, repoId string) (*bitbucket.Permission, error) {
	if principal.Id.ResourceType == resourceTypeUser.Id {
		userPermission, err := r.client.GetRepoUserPermission(
			ctx,
			workspaceId,
			repoId,
			principal.Id.Resource,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to get repository user permission: %w", err)
		}

		return &userPermission.Permission, nil
	} else if principal.Id.ResourceType == resourceTypeUserGroup.Id {
		_, groupSlug, err := DecomposeGroupId(principal.Id.Resource)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to grant repository permission: %w", err)
		}

		groupPermission, err := r.client.GetRepoGroupPermission(
			ctx,
			workspaceId,
			repoId,
			groupSlug,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to get repository group permission: %w", err)
		}

		return &groupPermission.Permission, nil
	}

	return nil, fmt.Errorf("bitbucket-connector: invalid principal resource type: %s", principal.Id.ResourceType)
}

func (r *repositoryResourceType) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principalIsUser := principal.Id.ResourceType == resourceTypeUser.Id
	principalIsGroup := principal.Id.ResourceType == resourceTypeUserGroup.Id

	if !principalIsUser && !principalIsGroup {
		l.Warn(
			"bitbucket-connector: only users and groups can be granted repository permissions",
			zap.String("principal_id", principal.Id.Resource),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: only users and groups can be granted repository permissions")
	}

	repositoryResourceId, slug, err := ParseEntitlementID(entitlement.Id)
	if err != nil {
		return nil, err
	}

	composedProjectId, repoId, err := DecomposeRepositoryId(repositoryResourceId.Resource)
	if err != nil {
		return nil, err
	}

	workspaceId, _, _, err := DecomposeProjectId(composedProjectId)
	if err != nil {
		return nil, err
	}

	permission, err := r.GetPermission(ctx, principal, workspaceId, repoId)
	if err != nil {
		return nil, err
	}

	// check if the permission is supported repository role
	if !contains(slug, repositoryRoles) {
		return nil, fmt.Errorf("bitbucket-connector: unsupported repository role: %s", entitlement.Slug)
	}

	// warn if the principal already has a repository permission
	if permission.Value != roleNone {
		l.Warn(
			"bitbucket-connector: principal already has a repository permission",
		)
	}

	// update the repository permission
	if principalIsUser {
		err := r.client.UpdateRepoUserPermission(
			ctx,
			workspaceId,
			repoId,
			principal.Id.Resource,
			slug,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to update repository user permission: %w", err)
		}
	} else if principalIsGroup {
		_, groupSlug, err := DecomposeGroupId(principal.Id.Resource)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to update repository permission: %w", err)
		}

		err = r.client.UpdateRepoGroupPermission(
			ctx,
			workspaceId,
			repoId,
			groupSlug,
			slug,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to update repository group permission: %w", err)
		}
	}

	return nil, nil
}

func (r *repositoryResourceType) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principal := grant.Principal
	entitlement := grant.Entitlement
	principalIsUser := principal.Id.ResourceType == resourceTypeUser.Id
	principalIsGroup := principal.Id.ResourceType == resourceTypeUserGroup.Id

	if !principalIsUser && !principalIsGroup {
		l.Warn(
			"bitbucket-connector: only users and groups can have repository permissions revoked",
			zap.String("principal_id", principal.Id.Resource),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: only users and groups can have repository permissions revoked")
	}

	repositoryResourceId, slug, err := ParseEntitlementID(entitlement.Id)
	if err != nil {
		return nil, err
	}

	composedProjectId, repoId, err := DecomposeRepositoryId(repositoryResourceId.Resource)
	if err != nil {
		return nil, err
	}

	workspaceId, _, _, err := DecomposeProjectId(composedProjectId)
	if err != nil {
		return nil, err
	}

	permission, err := r.GetPermission(
		ctx, principal, workspaceId, repoId)
	if err != nil {
		return nil, err
	}

	// check if the permission is supported repository role
	if !contains(slug, repositoryRoles) {
		return nil, fmt.Errorf("bitbucket-connector: unsupported repository role: %s", permission.Value)
	}

	// warn if the principal already doesnt have this repository permission
	if permission.Value == roleNone {
		l.Warn(
			"bitbucket-connector: principal already doesnt have this repository permission",
		)
	}

	// remove the repository permission
	if principalIsUser {
		err := r.client.DeleteRepoUserPermission(
			ctx,
			workspaceId,
			repoId,
			principal.Id.Resource,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to remove repository user permission: %w", err)
		}
	} else if principalIsGroup {
		_, groupSlug, err := DecomposeGroupId(principal.Id.Resource)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to remove repository user permission: %w", err)
		}

		err = r.client.DeleteRepoGroupPermission(
			ctx,
			workspaceId,
			repoId,
			groupSlug,
		)
		if err != nil {
			return nil, fmt.Errorf("bitbucket-connector: failed to remove repository group permission: %w", err)
		}
	}

	return nil, nil
}

func repositoryBuilder(client *bitbucket.Client) *repositoryResourceType {
	return &repositoryResourceType{
		resourceType: resourceTypeRepository,
		client:       client,
	}
}
