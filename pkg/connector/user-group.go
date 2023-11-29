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

	composedId := fmt.Sprintf("%s:%s", parentResourceID.Resource, userGroup.Slug)

	resource, err := rs.NewGroupResource(
		userGroup.Name,
		resourceTypeUserGroup,
		composedId,
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

	userGroups, err := ug.client.GetWorkspaceUserGroups(ctx, parentId.Resource)
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

	return rv, "", nil, nil
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
		user, err := ug.client.GetUser(ctx, id)
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

func (ug *userGroupResourceType) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	if principal.Id.ResourceType != resourceTypeUser.Id {
		l.Warn(
			"bitbucket-connector: only users can be granted group membership",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: only users can be granted group membership")
	}

	workspaceId, groupSlug, userId := principal.ParentResourceId.Resource, entitlement.Resource.Id.Resource, principal.Id.Resource

	// check if user is already a member of the group
	members, err := ug.client.GetUserGroupMembers(ctx, workspaceId, groupSlug)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get user group members: %w", err)
	}

	if isUserPresent(members, userId) {
		l.Warn(
			"bitbucket-connector: user is already a member of the group",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: user is already a member of the group")
	}

	// add user to the group
	err = ug.client.AddUserToGroup(ctx, workspaceId, groupSlug, userId)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to add user to user group: %w", err)
	}

	return nil, nil
}

func (ug *userGroupResourceType) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principal := grant.Principal
	entitlement := grant.Entitlement

	if principal.Id.ResourceType != resourceTypeUser.Id {
		l.Warn(
			"bitbucket-connector: only users can have group membership revoked",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: only users can have group membership revoked")
	}

	if entitlement.Resource == nil {
		l.Warn(
			"bitbucket-connector: entitlement does not have a resource",
			zap.String("entitlement_id", entitlement.Id),
		)

		return nil, fmt.Errorf("bitbucket-connector: entitlement does not have a resource")
	}

	parts := strings.Split(entitlement.Id, ":")
	if len(parts) != 4 {
		l.Warn(
			"bitbucket-connector: invalid resource id",
			zap.String("resource_id", entitlement.Id),
		)

		return nil, fmt.Errorf("bitbucket-connector: invalid resource id")
	}

	workspaceId, groupSlug := parts[1], parts[2]
	userId := principal.Id.Resource

	members, err := ug.client.GetUserGroupMembers(ctx, workspaceId, groupSlug)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to get user group members: %w", err)
	}

	if !isUserPresent(members, userId) {
		l.Warn(
			"bitbucket-connector: user is not a member of the group",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("bitbucket-connector: user is not a member of the group")
	}
	// add user to the group
	err = ug.client.RemoveUserFromGroup(ctx, workspaceId, groupSlug, userId)
	if err != nil {
		return nil, fmt.Errorf("bitbucket-connector: failed to remove user from user group: %w", err)
	}

	return nil, nil
}

func userGroupBuilder(client *bitbucket.Client) *userGroupResourceType {
	return &userGroupResourceType{
		resourceType: resourceTypeUserGroup,
		client:       client,
	}
}
