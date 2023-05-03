package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/ConductorOne/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	grant "github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
)

const repoEntitlement = "repository"

type projectResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (p *projectResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return p.resourceType
}

// Create a new connector resource for an BitBucket Project.
func projectResource(ctx context.Context, project *bitbucket.Project, parentResourceID *v2.ResourceId) (*v2.Resource, error) {
	profile := map[string]interface{}{
		"project_id":   project.Id,
		"project_name": project.Name,
		"project_key":  project.Key,
	}

	composedId := fmt.Sprintf("%s:%s", parentResourceID.Resource, project.Id)

	resource, err := rs.NewGroupResource(
		project.Name,
		resourceTypeProject,
		composedId,
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

	projects, nextToken, annotations, err := p.client.GetWorkspaceProjects(
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

		tResource, err := projectResource(ctx, &projectCopy, parentId)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, tResource)
	}

	return rv, pageToken, annotations, nil
}

func (p *projectResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement
	assignmentOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(resourceTypeRepository),
		ent.WithDisplayName(fmt.Sprintf("%s Project %s", resource.DisplayName, repoEntitlement)),
		ent.WithDescription(fmt.Sprintf("Access to %s project in BitBucket", resource.DisplayName)),
	}

	// create membership entitlement
	rv = append(rv, ent.NewAssignmentEntitlement(
		resource,
		repoEntitlement,
		assignmentOptions...,
	))

	return rv, "", nil, nil
}

func (p *projectResourceType) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeProject.Id})
	if err != nil {
		return nil, "", nil, err
	}

	parts := strings.Split(resource.Id.Resource, ":")
	if len(parts) != 2 {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: invalid project resource id: %s", resource.Id.Resource)
	}

	workspaceId, projectId := parts[0], parts[1]
	repos, nextToken, _, err := p.client.GetProjectRepos(
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

	pageToken, err := bag.NextToken(nextToken)
	if err != nil {
		return nil, "", nil, err
	}

	// create membership grants
	var rv []*v2.Grant
	for _, repo := range repos {
		repoCopy := repo
		rr, err := repositoryResource(ctx, &repoCopy, resource.ParentResourceId)
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

	return rv, pageToken, nil, nil
}

func projectBuilder(client *bitbucket.Client) *projectResourceType {
	return &projectResourceType{
		resourceType: resourceTypeProject,
		client:       client,
	}
}
