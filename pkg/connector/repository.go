package connector

import (
	"context"
	"fmt"

	"github.com/ConductorOne/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
)

type repositoryResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (r *repositoryResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return r.resourceType
}

// Create a new connector resource for an BitBucket Repository.
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
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeUser.Id})
	if err != nil {
		return nil, "", nil, err
	}

	// TODO: get the workspace id from the parent resource
	// workspaceId, err := getWorkspaceIdFromProject(parentId)
	// if err != nil {
	// 	return nil, "", nil, err
	// }

	repositories, nextToken, annotations, err := r.client.GetProjectRepos(
		ctx,
		"WORKSPACE_ID",
		parentId.Resource,
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

	return rv, pageToken, annotations, nil
}

func (r *repositoryResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (r *repositoryResourceType) Grants(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func repositoryBuilder(client *bitbucket.Client) *repositoryResourceType {
	return &repositoryResourceType{
		resourceType: resourceTypeRepository,
		client:       client,
	}
}
