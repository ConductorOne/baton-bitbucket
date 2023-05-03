package connector

import (
	"context"
	"fmt"

	"github.com/ConductorOne/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	grant "github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
)

const memberEntitlement = "member"

type workspaceResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (w *workspaceResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return w.resourceType
}

// Create a new connector resource for an BitBucket workspace.
func workspaceResource(ctx context.Context, workspace *bitbucket.Workspace) (*v2.Resource, error) {
	resource, err := rs.NewResource(
		fmt.Sprint(workspace.Slug),
		resourceTypeWorkspace,
		workspace.Id,
		rs.WithAnnotation(
			&v2.ChildResourceType{ResourceTypeId: resourceTypeUser.Id},
		),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

func (w *workspaceResourceType) List(ctx context.Context, _ *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	// parse the token
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeWorkspace.Id})
	if err != nil {
		return nil, "", nil, err
	}

	workspaces, nextToken, annotations, err := w.client.GetWorkspaces(
		ctx,
		bitbucket.PaginationVars{
			Limit: ResourcesPageSize,
			Page:  bag.PageToken(),
		},
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list workspace: %w", err)
	}

	pageToken, err := bag.NextToken(nextToken)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource
	for _, workspace := range workspaces {
		workspaceCopy := workspace

		wr, err := workspaceResource(ctx, &workspaceCopy)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, wr)
	}

	return rv, pageToken, annotations, nil
}

func (w *workspaceResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement

	assignmentOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(resourceTypeUser),
		ent.WithDisplayName(fmt.Sprintf("%s Workspace %s", resource.DisplayName, titleCaser.String(memberEntitlement))),
		ent.WithDescription(fmt.Sprintf("Workspace %s role in BitBucket", resource.DisplayName)),
	}

	// create the membership entitlement
	rv = append(rv, ent.NewAssignmentEntitlement(
		resource,
		memberEntitlement,
		assignmentOptions...,
	))

	return rv, "", nil, nil
}

func (w *workspaceResourceType) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	// parse the roleIds from the users
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeUser.Id})
	if err != nil {
		return nil, "", nil, err
	}

	users, nextToken, annotations, err := w.client.GetWorkspaceMembers(
		ctx,
		resource.Id.Resource,
		bitbucket.PaginationVars{Limit: ResourcesPageSize, Page: bag.PageToken()},
	)
	if err != nil {
		return nil, "", nil, err
	}

	pageToken, err := bag.NextToken(nextToken)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Grant
	for _, user := range users {
		userCopy := user
		u, err := userResource(ctx, &userCopy, nil)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(
			rv,
			grant.NewGrant(
				resource,
				memberEntitlement,
				u.Id,
			),
		)
	}

	return rv, pageToken, annotations, nil
}

func workspaceBuilder(client *bitbucket.Client) *workspaceResourceType {
	return &workspaceResourceType{
		resourceType: resourceTypeWorkspace,
		client:       client,
	}
}
