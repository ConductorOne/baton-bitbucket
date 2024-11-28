package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	mapset "github.com/deckarep/golang-set/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	BaseURL                        = "https://api.bitbucket.org"
	WorkspacesBaseURL              = "/2.0/workspaces"
	WorkspaceBaseURL               = "/2.0/workspaces/%s"
	WorkspaceMembersBaseURL        = "/2.0/workspaces/%s/members"
	WorkspaceProjectsBaseURL       = "/2.0/workspaces/%s/projects"
	ProjectRepositoriesBaseURL     = "/2.0/repositories/%s"
	UserBaseURL                    = "/2.0/users/%s"
	CurrentUserBaseURL             = "/2.0/user"
	WorkspaceUserGroupsBaseURL     = "/1.0/groups/%s"
	UserGroupMembersBaseURL        = "/1.0/groups/%s/%s/members"
	GroupMemberModifyBaseURL       = "/1.0/groups/%s/%s/members/%s"
	ProjectGroupPermissionsBaseURL = "/1.0/workspaces/%s/projects/%s/permissions-config/groups"
	ProjectGroupPermissionBaseURL  = "/1.0/workspaces/%s/projects/%s/permissions-config/groups/%s"
	ProjectUserPermissionsBaseURL  = "/1.0/workspaces/%s/projects/%s/permissions-config/users"
	ProjectUserPermissionBaseURL   = "/1.0/workspaces/%s/projects/%s/permissions-config/users/%s"
	RepoGroupPermissionsBaseURL    = "/2.0/repositories/%s/%s/permissions-config/groups"
	RepoGroupPermissionBaseURL     = "/2.0/repositories/%s/%s/permissions-config/groups/%s"
	RepoUserPermissionsBaseURL     = "/2.0/repositories/%s/%s/permissions-config/users"
	RepoUserPermissionBaseURL      = "/2.0/repositories/%s/%s/permissions-config/users/%s"
	PageSizeDefault                = 50
)

type Client struct {
	baseUrl      *url.URL
	wrapper      *uhttp.BaseHttpClient
	scope        Scope
	workspaceIDs mapset.Set[string]
}

func NewClient(ctx context.Context, httpClient *http.Client) (*Client, error) {
	wrapper, err := uhttp.NewBaseHttpClientWithContext(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(BaseURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseUrl:      baseUrl,
		wrapper:      wrapper,
		workspaceIDs: mapset.NewSet[string](),
	}, nil
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type ListResponse[T any] struct {
	Values []T `json:"values"`
	PaginationData
}

type UpdatePermissionPayload struct {
	Permission string `json:"permission"`
}

func (c *Client) SetupUserScope(userId string) {
	c.scope = &UserScoped{
		Username: userId,
	}
}

func (c *Client) SetupWorkspaceScope(workspaceId string) {
	c.scope = &WorkspaceScoped{
		Workspace: workspaceId,
	}
}

func (c *Client) IsUserScoped() bool {
	_, ok := c.scope.(*UserScoped)
	return ok
}

func (c *Client) IsWorkspaceScoped() bool {
	_, ok := c.scope.(*WorkspaceScoped)
	return ok
}

// WorkspaceId If client have access only to one workspace, method `WorkspaceId`
// returns that id otherwise it returns error.
func (c *Client) WorkspaceId() (string, error) {
	if c.IsWorkspaceScoped() {
		return c.scope.(*WorkspaceScoped).Workspace, nil
	}
	return "", status.Error(codes.InvalidArgument, "client is not workspace scoped")
}

// GetWorkspaces lists all workspaces current user belongs to.
func (c *Client) GetWorkspaces(ctx context.Context, getWorkspacesVars PaginationVars) ([]Workspace, string, error) {
	var workspacesResponse ListResponse[Workspace]
	err := c.get(
		ctx,
		c.getUrl(WorkspacesBaseURL),
		&workspacesResponse,
		[]QueryParam{
			&getWorkspacesVars,
			prepareFilters(""),
		},
	)
	if err != nil {
		return nil, "", err
	}
	workspacesResponse.Values, err = c.filterWorkspaces(ctx, workspacesResponse.Values)
	return HandlePagination(workspacesResponse, err)
}

// GetAllWorkspaces lists all workspaces looping through all pages.
func (c *Client) GetAllWorkspaces(ctx context.Context) ([]Workspace, error) {
	var allWorkspaces []Workspace
	var next string

	for {
		pagination := PaginationVars{
			Limit: PageSizeDefault,
			Page:  next,
		}

		workspaces, nextPage, err := c.GetWorkspaces(ctx, pagination)
		if err != nil {
			return nil, err
		}

		allWorkspaces = append(allWorkspaces, workspaces...)
		next = nextPage

		if next == "" {
			break
		}
	}

	return allWorkspaces, nil
}

// GetWorkspace get specific workspace based on provided id.
func (c *Client) GetWorkspace(ctx context.Context, workspaceId string) (*Workspace, error) {
	var workspaceResponse Workspace
	err := c.get(
		ctx,
		c.getUrl(WorkspaceBaseURL, workspaceId),
		&workspaceResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)
	if err != nil {
		if isPermissionDeniedErr(err) {
			return nil, status.Error(codes.PermissionDenied, "missing permission to get workspace")
		}
		return nil, err
	}

	return &workspaceResponse, nil
}

// GetWorkspaceMembers lists all users that belong under specified workspace.
func (c *Client) GetWorkspaceMembers(
	ctx context.Context,
	workspaceId string,
	getWorkspacesVars PaginationVars,
) ([]User, string, error) {
	var workspaceMembersResponse ListResponse[WorkspaceMember]
	err := c.get(
		ctx,
		c.getUrl(WorkspaceMembersBaseURL, workspaceId),
		&workspaceMembersResponse,
		[]QueryParam{
			&getWorkspacesVars,
			prepareFilters("", "-*.workspace"),
		},
	)
	if err != nil {
		return nil, "", err
	}

	members, page, _ := HandlePagination(workspaceMembersResponse, nil)

	return mapUsers(members), page, nil
}

// GetWorkspaceUserGroups lists all user groups that belong under specified workspace (This method is supported only for v1 API).
func (c *Client) GetWorkspaceUserGroups(ctx context.Context, workspaceId string) ([]UserGroup, error) {
	var workspaceUserGroupsResponse []UserGroup
	err := c.get(
		ctx,
		c.getUrl(WorkspaceUserGroupsBaseURL, workspaceId),
		&workspaceUserGroupsResponse,
		nil,
	)

	if err != nil {
		return nil, err
	}

	return workspaceUserGroupsResponse, nil
}

// GetUserGroupMembers lists all members that belong in specified user group (This method is supported only for v1 API).
func (c *Client) GetUserGroupMembers(ctx context.Context, workspaceId string, groupSlug string) ([]User, error) {
	var userGroupMembersResponse []User
	err := c.get(
		ctx,
		c.getUrl(UserGroupMembersBaseURL, workspaceId, groupSlug),
		&userGroupMembersResponse,
		nil,
	)

	if err != nil {
		return nil, err
	}

	return userGroupMembersResponse, nil
}

// AddUserToGroup adds new member under specified user group (This method is supported only for v1 API).
func (c *Client) AddUserToGroup(ctx context.Context, workspaceId string, groupSlug string, userId string) error {
	return c.put(
		ctx,
		c.getUrl(
			GroupMemberModifyBaseURL,
			workspaceId,
			groupSlug,
			userId,
		),
		struct{}{}, // required empty body
		nil,
		nil,
	)
}

// RemoveUserFromGroup removes member from specified user group (This method is supported only for v1 API).
func (c *Client) RemoveUserFromGroup(
	ctx context.Context,
	workspaceId string,
	groupSlug string,
	userId string,
) error {
	return c.delete(
		ctx,
		c.getUrl(
			GroupMemberModifyBaseURL,
			workspaceId,
			groupSlug,
			userId,
		),
	)
}

// GetCurrentUser get information about currently logged in user or team.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	var userResponse User
	err := c.get(
		ctx,
		c.getUrl(CurrentUserBaseURL),
		&userResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)
	return &userResponse, err
}

// GetUser get detail information about specified user.
func (c *Client) GetUser(ctx context.Context, userId string) (*User, error) {
	var userResponse User
	err := c.get(
		ctx,
		c.getUrl(UserBaseURL, userId),
		&userResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)
	return &userResponse, err
}

// GetWorkspaceProjects lists all projects that belong under specified workspace.
func (c *Client) GetWorkspaceProjects(
	ctx context.Context,
	workspaceId string,
	getWorkspaceProjectsVars PaginationVars,
) ([]Project, string, error) {
	var workspaceProjectsResponse ListResponse[Project]
	err := c.get(
		ctx,
		c.getUrl(WorkspaceProjectsBaseURL, workspaceId),
		&workspaceProjectsResponse,
		[]QueryParam{
			&getWorkspaceProjectsVars,
			prepareFilters("", "-*.workspace", "-*.owner"),
		},
	)
	return HandlePagination(workspaceProjectsResponse, err)
}

// GetAllWorkspaceProjects lists all projects looping through all pages.
func (c *Client) GetAllWorkspaceProjects(ctx context.Context, workspaceId string) ([]Project, error) {
	var allProjects []Project
	var next string

	for {
		pagination := PaginationVars{
			Limit: PageSizeDefault,
			Page:  next,
		}

		projects, nextPage, err := c.GetWorkspaceProjects(ctx, workspaceId, pagination)
		if err != nil {
			return nil, err
		}

		allProjects = append(allProjects, projects...)
		next = nextPage

		if next == "" {
			break
		}
	}

	return allProjects, nil
}

// GetProjectRepos lists all repositories that belong under specified project (which belongs under specified workspace).
func (c *Client) GetProjectRepos(
	ctx context.Context,
	workspaceId string,
	projectId string,
	getProjectReposVars PaginationVars,
) ([]Repository, string, error) {
	var projectRepositoriesResponse ListResponse[Repository]
	err := c.get(
		ctx,
		c.getUrl(ProjectRepositoriesBaseURL, workspaceId),
		&projectRepositoriesResponse,
		[]QueryParam{
			&getProjectReposVars,
			prepareFilters(
				fmt.Sprintf("project.uuid=\"%s\"", projectId),
				"-*.workspace",
				"-*.owner",
			),
		},
	)
	return HandlePagination(projectRepositoriesResponse, err)
}

// GetAllProjectRepos lists all repositories looping through all pages.
func (c *Client) GetAllProjectRepos(ctx context.Context, workspaceId string, projectId string) ([]Repository, error) {
	var allRepos []Repository
	var next string

	for {
		pagination := PaginationVars{
			Limit: PageSizeDefault,
			Page:  next,
		}

		repos, nextPage, err := c.GetProjectRepos(ctx, workspaceId, projectId, pagination)
		if err != nil {
			return nil, err
		}

		allRepos = append(allRepos, repos...)
		next = nextPage

		if next == "" {
			break
		}
	}

	return allRepos, nil
}

// GetProjectGroupPermissions lists all group permissions that belong under specified project.
func (c *Client) GetProjectGroupPermissions(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	getPermissionsVars PaginationVars,
) ([]GroupPermission, string, error) {
	var projectGroupPermissionsResponse ListResponse[GroupPermission]
	err := c.get(
		ctx,
		c.getUrl(ProjectGroupPermissionsBaseURL, workspaceId, projectKey),
		&projectGroupPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)
	return HandlePagination(projectGroupPermissionsResponse, err)
}

// GetProjectGroupPermission returns group permission of specific group under provided project.
func (c *Client) GetProjectGroupPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	groupSlug string,
) (*GroupPermission, error) {
	var projectGroupPermissionsResponse GroupPermission
	err := c.get(
		ctx,
		c.getUrl(
			ProjectGroupPermissionBaseURL,
			workspaceId,
			projectKey,
			groupSlug,
		),
		&projectGroupPermissionsResponse,
		[]QueryParam{
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)
	return &projectGroupPermissionsResponse, err
}

// UpdateProjectGroupPermission updates group permission of specific group under provided project.
func (c *Client) UpdateProjectGroupPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	groupSlug string,
	permission string,
) error {
	return c.put(
		ctx,
		c.getUrl(
			ProjectGroupPermissionBaseURL,
			workspaceId,
			projectKey,
			groupSlug,
		),
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)
}

// DeleteProjectGroupPermission removes group permission of specific group under provided project.
func (c *Client) DeleteProjectGroupPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	groupSlug string,
) error {
	return c.delete(
		ctx,
		c.getUrl(
			ProjectGroupPermissionBaseURL,
			workspaceId,
			projectKey,
			groupSlug,
		),
	)
}

// GetProjectUserPermissions lists all user permissions that belong under specified project.
func (c *Client) GetProjectUserPermissions(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	getPermissionsVars PaginationVars,
) ([]UserPermission, string, error) {
	var projectUserPermissionsResponse ListResponse[UserPermission]
	err := c.get(
		ctx,
		c.getUrl(ProjectUserPermissionsBaseURL, workspaceId, projectKey),
		&projectUserPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters(""),
		},
	)
	return HandlePagination(projectUserPermissionsResponse, err)
}

// GetProjectUserPermission returns user permission of specific user under provided project.
func (c *Client) GetProjectUserPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	userId string,
) (*UserPermission, error) {
	var projectUserPermissionsResponse UserPermission
	err := c.get(
		ctx,
		c.getUrl(
			ProjectUserPermissionBaseURL,
			workspaceId,
			projectKey,
			userId,
		),
		&projectUserPermissionsResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)
	return &projectUserPermissionsResponse, err
}

// UpdateProjectUserPermission updates user permission of specific user under provided project.
func (c *Client) UpdateProjectUserPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	userId string,
	permission string,
) error {
	return c.put(
		ctx,
		c.getUrl(
			ProjectUserPermissionBaseURL,
			workspaceId,
			projectKey,
			userId,
		),
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)
}

// DeleteProjectUserPermission removes user permission of specific user under provided project.
func (c *Client) DeleteProjectUserPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	userId string,
) error {
	return c.delete(
		ctx,
		c.getUrl(
			ProjectUserPermissionBaseURL,
			workspaceId,
			projectKey,
			userId,
		),
	)
}

// GetRepositoryGroupPermissions lists all group permissions that belong under specified repository.
func (c *Client) GetRepositoryGroupPermissions(
	ctx context.Context,
	workspaceId string,
	repoId string,
	getPermissionsVars PaginationVars,
) ([]GroupPermission, string, error) {
	var repositoryGroupPermissionsResponse ListResponse[GroupPermission]
	err := c.get(
		ctx,
		c.getUrl(RepoGroupPermissionsBaseURL, workspaceId, repoId),
		&repositoryGroupPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)

	return HandlePagination(repositoryGroupPermissionsResponse, err)
}

// GetRepoGroupPermission returns group permission of specific group under provided repository.
func (c *Client) GetRepoGroupPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	groupSlug string,
) (*GroupPermission, error) {
	var repoGroupPermissionsResponse GroupPermission
	err := c.get(
		ctx,
		c.getUrl(RepoGroupPermissionBaseURL, workspaceId, repoId, groupSlug),
		&repoGroupPermissionsResponse,
		[]QueryParam{
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)
	return &repoGroupPermissionsResponse, err
}

// UpdateRepoGroupPermission updates group permission of specific group under provided repository.
func (c *Client) UpdateRepoGroupPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	groupSlug string,
	permission string,
) error {
	return c.put(
		ctx,
		c.getUrl(RepoGroupPermissionBaseURL, workspaceId, repoId, groupSlug),
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)
}

// DeleteRepoGroupPermission removes group permission of specific group under provided repository.
func (c *Client) DeleteRepoGroupPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	groupSlug string,
) error {
	return c.delete(
		ctx,
		c.getUrl(RepoGroupPermissionBaseURL, workspaceId, repoId, groupSlug),
	)
}

// GetRepositoryUserPermissions lists all user permissions that belong under specified repository.
func (c *Client) GetRepositoryUserPermissions(
	ctx context.Context,
	workspaceId string,
	repoId string,
	getPermissionsVars PaginationVars,
) ([]UserPermission, string, error) {
	var repositoryUserPermissionsResponse ListResponse[UserPermission]
	err := c.get(
		ctx,
		c.getUrl(RepoUserPermissionsBaseURL, workspaceId, repoId),
		&repositoryUserPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters(""),
		},
	)
	return HandlePagination(repositoryUserPermissionsResponse, err)
}

// GetRepoUserPermission returns user permission of specific user under provided repository.
func (c *Client) GetRepoUserPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	userId string,
) (*UserPermission, error) {
	var repoUserPermissionsResponse UserPermission
	err := c.get(
		ctx,
		c.getUrl(
			RepoUserPermissionBaseURL,
			workspaceId,
			repoId,
			userId,
		),
		&repoUserPermissionsResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)
	return &repoUserPermissionsResponse, err
}

// UpdateRepoUserPermission updates user permission of specific user under provided repository.
func (c *Client) UpdateRepoUserPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	userId string,
	permission string,
) error {
	return c.put(
		ctx,
		c.getUrl(
			RepoUserPermissionBaseURL,
			workspaceId,
			repoId,
			userId,
		),
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)
}

// DeleteRepoUserPermission removes user permission of specific user under provided repository.
func (c *Client) DeleteRepoUserPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	userId string,
) error {
	return c.delete(
		ctx,
		c.getUrl(
			RepoUserPermissionBaseURL,
			workspaceId,
			repoId,
			userId,
		),
	)
}

func mapUsers(members []WorkspaceMember) []User {
	var users []User

	for _, member := range members {
		users = append(users, member.User)
	}

	return users
}
