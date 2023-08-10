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
)

var repositoryRoles = []string{roleRead, roleWrite, roleAdmin}

type repositoryResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (r *repositoryResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return r.resourceType
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
		repository.Id,
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

	parts := strings.Split(parentId.Resource, ":")
	if len(parts) != 2 {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: invalid parent resource id: %s", parentId.Resource)
	}

	workspaceId, projectId := parts[0], parts[1]
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
			ent.WithDescription(fmt.Sprintf("%s access to %s repository in Bitbucket", titleCaser.String(role), resource.DisplayName)),
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

	parts := strings.Split(resource.ParentResourceId.Resource, ":")
	if len(parts) != 2 {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: invalid parent project resource id: %s", resource.Id.Resource)
	}

	workspaceId := parts[0]
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
			resource.Id.Resource,
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

			gr, err := userGroupResource(ctx, &groupCopy, resource.ParentResourceId)
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
			resource.Id.Resource,
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

			ur, err := userResource(ctx, &memberCopy, resource.ParentResourceId)
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

func repositoryBuilder(client *bitbucket.Client) *repositoryResourceType {
	return &repositoryResourceType{
		resourceType: resourceTypeRepository,
		client:       client,
	}
}
