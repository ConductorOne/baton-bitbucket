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

type userGroupResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (ug *userGroupResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return ug.resourceType
}

// Create a new connector resource for an Bitbucket UserGroup.
func userGroupResource(ctx context.Context, userGroup *bitbucket.UserGroup, parentResourceID *v2.ResourceId) (*v2.Resource, error) {
	userIdsTotal := len(userGroup.Members)
	profile := map[string]interface{}{
		"userGroup_name":       userGroup.Name,
		"userGroup_slug":       userGroup.Slug,
		"userGroup_permission": userGroup.Permission,
	}

	if userIdsTotal > 0 {
		userIds := mapUserIds(userGroup.Members)

		profile["userGroup_members"] = strings.Join(userIds, ",")
	}

	resource, err := rs.NewGroupResource(
		userGroup.Name,
		resourceTypeUserGroup,
		userGroup.Slug,
		[]rs.GroupTraitOption{rs.WithGroupProfile(profile)},
		rs.WithParentResourceID(parentResourceID),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

func (ug *userGroupResourceType) List(ctx context.Context, parentId *v2.ResourceId, _ *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	if parentId == nil {
		return nil, "", nil, nil
	}

	userGroups, annotations, err := ug.client.GetWorkspaceUserGroups(ctx, parentId.Resource)
	if err != nil {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list userGroups: %w", err)
	}

	var rv []*v2.Resource
	for _, userGroup := range userGroups {
		userGroupCopy := userGroup

		gr, err := userGroupResource(ctx, &userGroupCopy, parentId)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, gr)
	}

	return rv, "", annotations, nil
}

func (ug *userGroupResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement
	assignmentOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(resourceTypeUser),
		ent.WithDisplayName(fmt.Sprintf("%s UserGroup %s", resource.DisplayName, memberEntitlement)),
		ent.WithDescription(fmt.Sprintf("Access to %s userGroup in Bitbucket", resource.DisplayName)),
	}

	// create membership entitlement
	rv = append(rv, ent.NewAssignmentEntitlement(
		resource,
		memberEntitlement,
		assignmentOptions...,
	))

	return rv, "", nil, nil
}

func (ug *userGroupResourceType) Grants(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	userGroupTrait, err := rs.GetGroupTrait(resource)
	if err != nil {
		return nil, "", nil, err
	}

	userIdsString, ok := rs.GetProfileStringValue(userGroupTrait.Profile, "userGroup_members")
	if !ok {
		return nil, "", nil, nil
	}

	userIds := strings.Split(userIdsString, ",")

	// create membership grants
	var rv []*v2.Grant
	for _, id := range userIds {
		user, _, err := ug.client.GetUser(ctx, id)
		if err != nil {
			return nil, "", nil, err
		}

		userCopy := user
		ur, err := userResource(ctx, userCopy, resource.ParentResourceId)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(
			rv,
			grant.NewGrant(
				resource,
				memberEntitlement,
				ur.Id,
			),
		)
	}

	return rv, "", nil, nil
}

func userGroupBuilder(client *bitbucket.Client) *userGroupResourceType {
	return &userGroupResourceType{
		resourceType: resourceTypeUserGroup,
		client:       client,
	}
}
