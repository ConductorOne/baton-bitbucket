package connector

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
)

type userResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client
}

func (u *userResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return u.resourceType
}

// Create a new connector resource for an Bitbucket user.
func userResource(ctx context.Context, user *bitbucket.User, parentResourceID *v2.ResourceId) (*v2.Resource, error) {
	firstName, lastName := splitFullName(user.Name)

	profile := map[string]interface{}{
		"first_name": firstName,
		"last_name":  lastName,
		"login":      user.Username,
		"user_id":    user.Id,
	}

	status := rs.WithStatus(v2.UserTrait_Status_STATUS_ENABLED)
	if user.Status != "active" {
		status = rs.WithStatus(v2.UserTrait_Status_STATUS_DISABLED)
	}

	userTraitOptions := []rs.UserTraitOption{
		rs.WithUserProfile(profile),
		status,
	}

	resource, err := rs.NewUserResource(
		user.Name,
		resourceTypeUser,
		user.Id,
		userTraitOptions,
		rs.WithParentResourceID(parentResourceID),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

func (u *userResourceType) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	if parentId == nil {
		return nil, "", nil, nil
	}

	// parse the token
	bag, err := parsePageToken(token.Token, &v2.ResourceId{ResourceType: resourceTypeUser.Id})
	if err != nil {
		return nil, "", nil, err
	}

	users, nextToken, err := u.client.GetWorkspaceMembers(
		ctx,
		parentId.Resource,
		bitbucket.PaginationVars{
			Limit: ResourcesPageSize,
			Page:  bag.PageToken(),
		},
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list user: %w", err)
	}

	pageToken, err := bag.NextToken(nextToken)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource
	for _, user := range users {
		// retrieve a user to get a status
		u, err := u.client.GetUser(ctx, user.Id)
		if err != nil {
			return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to get user: %w", err)
		}

		ur, err := userResource(ctx, u, parentId)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, ur)
	}

	return rv, pageToken, nil, nil
}

func (u *userResourceType) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (u *userResourceType) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func userBuilder(client *bitbucket.Client) *userResourceType {
	return &userResourceType{
		resourceType: resourceTypeUser,
		client:       client,
	}
}
