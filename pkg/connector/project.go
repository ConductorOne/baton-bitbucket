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
const (
	roleRead   = "read"
	roleWrite  = "write"
	roleCreate = "create-repo"
	roleAdmin  = "admin"
)

var projectPermissions = []string{roleRead, roleWrite, roleCreate, roleAdmin}

type projectResourceType struct {
	resourceType *v2.ResourceType
	client       *bitbucket.Client

	usersGranted map[string][]string
}

func (p *projectResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return p.resourceType
}

// Create a new connector resource for an Bitbucket Project.
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
			ent.WithGrantableTo(resourceTypeUser),
			ent.WithDisplayName(fmt.Sprintf("%s Project %s", resource.DisplayName, permission)),
			ent.WithDescription(fmt.Sprintf("%s access to %s project in Bitbucket", titleCaser.String(permission), resource.DisplayName)),
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

	parts := strings.Split(resource.Id.Resource, ":")
	if len(parts) != 2 {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: invalid project resource id: %s", resource.Id.Resource)
	}

	workspaceId, projectId := parts[0], parts[1]

	projectGroupTrait, err := rs.GetGroupTrait(resource)
	if err != nil {
		return nil, "", nil, err
	}

	projectKey, ok := rs.GetProfileStringValue(projectGroupTrait.Profile, "project_key")
	if !ok {
		return nil, "", nil, fmt.Errorf("bitbucket-connector: project_key not found in project group profile")
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

		err = bag.Next(nextToken)
		if err != nil {
			return nil, "", nil, err
		}

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

	// create a permission grant for each usergroup in the project
	case resourceTypeUserGroup.Id:
		permissions, nextToken, _, err := p.client.GetProjectGroupPermissions(
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

			members, _, err := p.client.GetUserGroupMembers(
				ctx,
				workspaceId,
				permission.Group.Slug,
			)
			if err != nil {
				return nil, "", nil, fmt.Errorf("bitbucket-connector: failed to list project group members: %w", err)
			}

			for _, member := range members {
				memberCopy := member

				// skip if already granted
				if contains(member.Id, p.usersGranted[permission.Value]) {
					continue
				}

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

				p.usersGranted[permission.Value] = append(p.usersGranted[permission.Value], member.Id)
			}
		}

	// create a permission grant for each user in the project
	case resourceTypeUser.Id:
		permissions, nextToken, _, err := p.client.GetProjectUserPermissions(
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

			// skip if already granted
			if contains(permission.User.Id, p.usersGranted[permission.Value]) {
				continue
			}

			ur, err := userResource(ctx, &permission.User, resource.ParentResourceId)
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

			p.usersGranted[permission.Value] = append(p.usersGranted[permission.Value], permission.User.Id)
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

func projectBuilder(client *bitbucket.Client) *projectResourceType {
	return &projectResourceType{
		resourceType: resourceTypeProject,
		client:       client,
		usersGranted: make(map[string][]string),
	}
}
