package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	V1BaseURL = "https://api.bitbucket.org/1.0/"
	BaseURL   = "https://api.bitbucket.org/2.0/"

	WorkspacesBaseURL          = BaseURL + "workspaces"
	WorkspaceBaseURL           = WorkspacesBaseURL + "/%s"
	WorkspaceMembersBaseURL    = WorkspacesBaseURL + "/%s/members"
	WorkspaceProjectsBaseURL   = WorkspacesBaseURL + "/%s/projects"
	ProjectRepositoriesBaseURL = BaseURL + "repositories/%s"
	UserBaseURL                = BaseURL + "users/%s"
	CurrentUserBaseURL         = BaseURL + "user"

	WorkspaceUserGroupsBaseURL = V1BaseURL + "groups/%s"
	UserGroupMembersBaseURL    = WorkspaceUserGroupsBaseURL + "/%s/members"
	GroupMemberModifyBaseURL   = WorkspaceUserGroupsBaseURL + "/%s/members/%s"

	ProjectPermissionsBaseURL      = WorkspacesBaseURL + "/%s/projects/%s/permissions-config"
	ProjectGroupPermissionsBaseURL = ProjectPermissionsBaseURL + "/groups"
	ProjectGroupPermissionBaseURL  = ProjectPermissionsBaseURL + "/groups/%s"
	ProjectUserPermissionsBaseURL  = ProjectPermissionsBaseURL + "/users"
	ProjectUserPermissionBaseURL   = ProjectPermissionsBaseURL + "/users/%s"

	RepoPermissionsBaseURL      = ProjectRepositoriesBaseURL + "/%s/permissions-config"
	RepoGroupPermissionsBaseURL = RepoPermissionsBaseURL + "/groups"
	RepoGroupPermissionBaseURL  = RepoPermissionsBaseURL + "/groups/%s"
	RepoUserPermissionsBaseURL  = RepoPermissionsBaseURL + "/users"
	RepoUserPermissionBaseURL   = RepoPermissionsBaseURL + "/users/%s"
)

type Client struct {
	wrapper      *uhttp.BaseHttpClient
	scope        Scope
	workspaceIDs map[string]bool
}

func NewClient(ctx context.Context, httpClient *http.Client) (*Client, error) {
	wrapper, err := uhttp.NewBaseHttpClientWithContext(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	return &Client{
		wrapper: wrapper,
	}, nil
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type ListResponse[T any] struct {
	Values []T `json:"values"`
	PaginationData
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (er *errorResponse) Message() string {
	return fmt.Sprintf("Error: %s", er.Error.Message)
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

// If client have access only to one workspace, method `WorkspaceId`
// returns that id otherwise it returns error.
func (c *Client) WorkspaceId() (string, error) {
	if c.IsWorkspaceScoped() {
		return c.scope.(*WorkspaceScoped).Workspace, nil
	} else {
		return "", status.Error(codes.InvalidArgument, "client is not workspace scoped")
	}
}

func isPermissionDeniedErr(err error) bool {
	e, ok := status.FromError(err)
	if ok && e.Code() == codes.PermissionDenied {
		return true
	}
	// In most cases the error code is unknown and the error message contains "status 403".
	if (!ok || e.Code() == codes.Unknown) && strings.Contains(err.Error(), "status 403") {
		return true
	}
	return false
}

func (c *Client) checkPermissions(ctx context.Context, workspace *Workspace) (bool, error) {
	l := ctxzap.Extract(ctx)
	logMissingPermission := func(obj string, err error) {
		l.Error(
			"missing permission to list object in workspace",
			zap.String("workspace", workspace.Slug),
			zap.String("workspace id", workspace.Id),
			zap.String("object", obj),
			zap.Error(err),
		)
	}
	paginationVars := PaginationVars{
		Limit: 1,
		Page:  "",
	}
	_, err := c.GetWorkspaceUserGroups(ctx, workspace.Id)
	if err != nil {
		if isPermissionDeniedErr(err) {
			logMissingPermission("userGroups", err)
			return false, nil
		}
		return false, err
	}
	_, _, err = c.GetWorkspaceMembers(ctx, workspace.Id, paginationVars)
	if err != nil {
		if isPermissionDeniedErr(err) {
			logMissingPermission("users", err)
			return false, nil
		}
		return false, err
	}
	_, _, err = c.GetWorkspaceProjects(ctx, workspace.Id, paginationVars)
	if err != nil {
		if isPermissionDeniedErr(err) {
			logMissingPermission("projects", err)
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Client) filterWorkspaces(ctx context.Context, workspaces []Workspace) ([]Workspace, error) {
	filteredWorkspaces := make([]Workspace, 0)

	for _, workspace := range workspaces {
		// We call this function in order to initialize the workspaceID's map. In that case we need to return all workspaces,
		// so they can be filtered and only the valid ones are set in the workspaceIds map.
		_, ok := c.workspaceIDs[workspace.Id]
		if c.workspaceIDs != nil && len(c.workspaceIDs) > 0 && !ok {
			continue
		}

		filteredWorkspaces = append(filteredWorkspaces, workspace)
	}

	return filteredWorkspaces, nil
}

// If client have access to multiple workspaces, method `WorkspaceIDs`
// returns list of workspace ids otherwise it returns error.
func (c *Client) SetWorkspaceIDs(ctx context.Context, workspaceIDs []string) error {
	if !c.IsUserScoped() {
		return status.Error(codes.InvalidArgument, "client is not user scoped")
	}
	c.workspaceIDs = make(map[string]bool)
	givenWorkspaceIDs := make(map[string]bool)
	for _, workspaceId := range workspaceIDs {
		givenWorkspaceIDs[workspaceId] = true
	}

	workspaces, err := c.GetAllWorkspaces(ctx)
	if err != nil {
		return err
	}

	for _, workspace := range workspaces {
		workspace := workspace
		if _, ok := givenWorkspaceIDs[workspace.Id]; !ok && len(givenWorkspaceIDs) > 0 {
			continue
		}
		ok, err := c.checkPermissions(ctx, &workspace)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		c.workspaceIDs[workspace.Id] = true
	}
	if len(c.workspaceIDs) == 0 {
		return status.Error(codes.Unauthenticated, "no authenticated workspaces found")
	}
	return nil
}

// GetWorkspaces lists all workspaces current user belongs to.
func (c *Client) GetWorkspaces(ctx context.Context, getWorkspacesVars PaginationVars) ([]Workspace, string, error) {
	urlAddress, err := url.Parse(WorkspacesBaseURL)
	if err != nil {
		return nil, "", err
	}

	var workspacesResponse ListResponse[Workspace]
	err = c.get(
		ctx,
		urlAddress,
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
	if err != nil {
		return nil, "", err
	}

	return handlePagination(workspacesResponse)
}

// GetAllWorkspaces lists all workspaces looping through all pages.
func (c *Client) GetAllWorkspaces(ctx context.Context) ([]Workspace, error) {
	var allWorkspaces []Workspace
	var next string

	for {
		pagination := PaginationVars{
			Limit: 50,
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
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(WorkspaceBaseURL, encodedWorkspaceId))
	if err != nil {
		return nil, err
	}

	var workspaceResponse Workspace
	err = c.get(
		ctx,
		urlAddress,
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
func (c *Client) GetWorkspaceMembers(ctx context.Context, workspaceId string, getWorkspacesVars PaginationVars) ([]User, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(WorkspaceMembersBaseURL, encodedWorkspaceId))
	if err != nil {
		return nil, "", err
	}

	var workspaceMembersResponse ListResponse[WorkspaceMember]
	err = c.get(
		ctx,
		urlAddress,
		&workspaceMembersResponse,
		[]QueryParam{
			&getWorkspacesVars,
			prepareFilters("", "-*.workspace"),
		},
	)
	if err != nil {
		return nil, "", err
	}

	members, page, _ := handlePagination(workspaceMembersResponse)

	return mapUsers(members), page, nil
}

// GetWorkspaceUserGroups lists all user groups that belong under specified workspace (This method is supported only for v1 API).
func (c *Client) GetWorkspaceUserGroups(ctx context.Context, workspaceId string) ([]UserGroup, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(WorkspaceUserGroupsBaseURL, encodedWorkspaceId))
	if err != nil {
		return nil, err
	}

	var workspaceUserGroupsResponse []UserGroup
	err = c.get(
		ctx,
		urlAddress,
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
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(UserGroupMembersBaseURL, encodedWorkspaceId, groupSlug))
	if err != nil {
		return nil, err
	}

	var userGroupMembersResponse []User
	err = c.get(
		ctx,
		urlAddress,
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
	encodedWorkspaceId := url.PathEscape(workspaceId)
	encodedUserId := url.PathEscape(userId)
	urlAddress, err := url.Parse(fmt.Sprintf(GroupMemberModifyBaseURL, encodedWorkspaceId, groupSlug, encodedUserId))
	if err != nil {
		return err
	}

	err = c.put(
		ctx,
		urlAddress,
		struct{}{}, // required empty body
		nil,
		nil,
	)
	if err != nil {
		return err
	}

	return nil
}

// RemoveUserFromGroup removes member from specified user group (This method is supported only for v1 API).
func (c *Client) RemoveUserFromGroup(ctx context.Context, workspaceId string, groupSlug string, userId string) error {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	encodedUserId := url.PathEscape(userId)
	urlAddress, err := url.Parse(fmt.Sprintf(GroupMemberModifyBaseURL, encodedWorkspaceId, groupSlug, encodedUserId))
	if err != nil {
		return err
	}

	err = c.delete(ctx, urlAddress)
	if err != nil {
		return err
	}

	return nil
}

// GetCurrentUser get information about currently logged in user or team.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	urlAddress, err := url.Parse(CurrentUserBaseURL)
	if err != nil {
		return nil, err
	}

	var userResponse User
	err = c.get(
		ctx,
		urlAddress,
		&userResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, err
	}

	return &userResponse, nil
}

// GetUser get detail information about specified user.
func (c *Client) GetUser(ctx context.Context, userId string) (*User, error) {
	encodedUserId := url.PathEscape(userId)
	urlAddress, err := url.Parse(fmt.Sprintf(UserBaseURL, encodedUserId))
	if err != nil {
		return nil, err
	}

	var userResponse User
	err = c.get(
		ctx,
		urlAddress,
		&userResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, err
	}

	return &userResponse, nil
}

// GetWorkspaceProjects lists all projects that belong under specified workspace.
func (c *Client) GetWorkspaceProjects(ctx context.Context, workspaceId string, getWorkspaceProjectsVars PaginationVars) ([]Project, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(WorkspaceProjectsBaseURL, encodedWorkspaceId))
	if err != nil {
		return nil, "", err
	}

	var workspaceProjectsResponse ListResponse[Project]
	err = c.get(
		ctx,
		urlAddress,
		&workspaceProjectsResponse,
		[]QueryParam{
			&getWorkspaceProjectsVars,
			prepareFilters("", "-*.workspace", "-*.owner"),
		},
	)

	if err != nil {
		return nil, "", err
	}

	return handlePagination(workspaceProjectsResponse)
}

// GetAllWorkspaceProjects lists all projects looping through all pages.
func (c *Client) GetAllWorkspaceProjects(ctx context.Context, workspaceId string) ([]Project, error) {
	var allProjects []Project
	var next string

	for {
		pagination := PaginationVars{
			Limit: 50,
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
func (c *Client) GetProjectRepos(ctx context.Context, workspaceId string, projectId string, getProjectReposVars PaginationVars) ([]Repository, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectRepositoriesBaseURL, encodedWorkspaceId))
	if err != nil {
		return nil, "", err
	}

	var projectRepositoriesResponse ListResponse[Repository]
	err = c.get(
		ctx,
		urlAddress,
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

	if err != nil {
		return nil, "", err
	}

	return handlePagination(projectRepositoriesResponse)
}

// GetAllProjectRepos lists all repositories looping through all pages.
func (c *Client) GetAllProjectRepos(ctx context.Context, workspaceId string, projectId string) ([]Repository, error) {
	var allRepos []Repository
	var next string

	for {
		pagination := PaginationVars{
			Limit: 50,
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
func (c *Client) GetProjectGroupPermissions(ctx context.Context, workspaceId string, projectKey string, getPermissionsVars PaginationVars) ([]GroupPermission, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectGroupPermissionsBaseURL, encodedWorkspaceId, projectKey))
	if err != nil {
		return nil, "", err
	}

	var projectGroupPermissionsResponse ListResponse[GroupPermission]
	err = c.get(
		ctx,
		urlAddress,
		&projectGroupPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)

	if err != nil {
		return nil, "", err
	}

	return handlePagination(projectGroupPermissionsResponse)
}

// GetProjectGroupPermission returns group permission of specific group under provided project.
func (c *Client) GetProjectGroupPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	groupSlug string,
) (*GroupPermission, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectGroupPermissionBaseURL, encodedWorkspaceId, projectKey, groupSlug))
	if err != nil {
		return nil, err
	}

	var projectGroupPermissionsResponse GroupPermission
	err = c.get(
		ctx,
		urlAddress,
		&projectGroupPermissionsResponse,
		[]QueryParam{
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)

	if err != nil {
		return nil, err
	}

	return &projectGroupPermissionsResponse, nil
}

// UpdateProjectGroupPermission updates group permission of specific group under provided project.
func (c *Client) UpdateProjectGroupPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	groupSlug string,
	permission string,
) error {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectGroupPermissionBaseURL, encodedWorkspaceId, projectKey, groupSlug))
	if err != nil {
		return err
	}

	err = c.put(
		ctx,
		urlAddress,
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// DeleteProjectGroupPermission removes group permission of specific group under provided project.
func (c *Client) DeleteProjectGroupPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	groupSlug string,
) error {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectGroupPermissionBaseURL, encodedWorkspaceId, projectKey, groupSlug))
	if err != nil {
		return err
	}

	err = c.delete(ctx, urlAddress)
	if err != nil {
		return err
	}

	return nil
}

// GetProjectUserPermissions lists all user permissions that belong under specified project.
func (c *Client) GetProjectUserPermissions(ctx context.Context, workspaceId string, projectKey string, getPermissionsVars PaginationVars) ([]UserPermission, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectUserPermissionsBaseURL, encodedWorkspaceId, projectKey))
	if err != nil {
		return nil, "", err
	}

	var projectUserPermissionsResponse ListResponse[UserPermission]
	err = c.get(
		ctx,
		urlAddress,
		&projectUserPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, "", err
	}

	return handlePagination(projectUserPermissionsResponse)
}

// GetProjectUserPermission returns user permission of specific user under provided project.
func (c *Client) GetProjectUserPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	userId string,
) (*UserPermission, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	encodedUserId := url.PathEscape(userId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectUserPermissionBaseURL, encodedWorkspaceId, projectKey, encodedUserId))
	if err != nil {
		return nil, err
	}

	var projectUserPermissionsResponse UserPermission
	err = c.get(
		ctx,
		urlAddress,
		&projectUserPermissionsResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, err
	}

	return &projectUserPermissionsResponse, nil
}

// UpdateProjectUserPermission updates user permission of specific user under provided project.
func (c *Client) UpdateProjectUserPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	userId string,
	permission string,
) error {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	encodedUserId := url.PathEscape(userId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectUserPermissionBaseURL, encodedWorkspaceId, projectKey, encodedUserId))
	if err != nil {
		return err
	}

	err = c.put(
		ctx,
		urlAddress,
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// DeleteProjectUserPermission removes user permission of specific user under provided project.
func (c *Client) DeleteProjectUserPermission(
	ctx context.Context,
	workspaceId string,
	projectKey string,
	userId string,
) error {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	encodedUserId := url.PathEscape(userId)
	urlAddress, err := url.Parse(fmt.Sprintf(ProjectUserPermissionBaseURL, encodedWorkspaceId, projectKey, encodedUserId))
	if err != nil {
		return err
	}

	err = c.delete(ctx, urlAddress)
	if err != nil {
		return err
	}

	return nil
}

// GetRepositoryGroupPermissions lists all group permissions that belong under specified repository.
func (c *Client) GetRepositoryGroupPermissions(ctx context.Context, workspaceId string, repoId string, getPermissionsVars PaginationVars) ([]GroupPermission, string, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoGroupPermissionsBaseURL, encodedWorkspaceId, encodedRepoId))
	if err != nil {
		return nil, "", err
	}

	var repositoryGroupPermissionsResponse ListResponse[GroupPermission]
	err = c.get(
		ctx,
		urlAddress,
		&repositoryGroupPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)

	if err != nil {
		return nil, "", err
	}

	return handlePagination(repositoryGroupPermissionsResponse)
}

// GetRepoGroupPermission returns group permission of specific group under provided repository.
func (c *Client) GetRepoGroupPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	groupSlug string,
) (*GroupPermission, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoGroupPermissionBaseURL, encodedWorkspaceId, encodedRepoId, groupSlug))
	if err != nil {
		return nil, err
	}

	var repoGroupPermissionsResponse GroupPermission
	err = c.get(
		ctx,
		urlAddress,
		&repoGroupPermissionsResponse,
		[]QueryParam{
			prepareFilters("", "-*.*.workspace", "-*.*.owner"),
		},
	)

	if err != nil {
		return nil, err
	}

	return &repoGroupPermissionsResponse, nil
}

// UpdateRepoGroupPermission updates group permission of specific group under provided repository.
func (c *Client) UpdateRepoGroupPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	groupSlug string,
	permission string,
) error {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoGroupPermissionBaseURL, encodedWorkspaceId, encodedRepoId, groupSlug))
	if err != nil {
		return err
	}

	err = c.put(
		ctx,
		urlAddress,
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// DeleteRepoGroupPermission removes group permission of specific group under provided repository.
func (c *Client) DeleteRepoGroupPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	groupSlug string,
) error {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoGroupPermissionBaseURL, encodedWorkspaceId, encodedRepoId, groupSlug))
	if err != nil {
		return err
	}

	err = c.delete(ctx, urlAddress)

	if err != nil {
		return err
	}

	return nil
}

// GetRepositoryUserPermissions lists all user permissions that belong under specified repository.
func (c *Client) GetRepositoryUserPermissions(ctx context.Context, workspaceId string, repoId string, getPermissionsVars PaginationVars) ([]UserPermission, string, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoUserPermissionsBaseURL, encodedWorkspaceId, encodedRepoId))
	if err != nil {
		return nil, "", err
	}

	var repositoryUserPermissionsResponse ListResponse[UserPermission]
	err = c.get(
		ctx,
		urlAddress,
		&repositoryUserPermissionsResponse,
		[]QueryParam{
			&getPermissionsVars,
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, "", err
	}

	return handlePagination(repositoryUserPermissionsResponse)
}

// GetRepoUserPermission returns user permission of specific user under provided repository.
func (c *Client) GetRepoUserPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	userId string,
) (*UserPermission, error) {
	encodedWorkspaceId, encodedUserId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(userId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoUserPermissionBaseURL, encodedWorkspaceId, encodedRepoId, encodedUserId))
	if err != nil {
		return nil, err
	}

	var repoUserPermissionsResponse UserPermission
	err = c.get(
		ctx,
		urlAddress,
		&repoUserPermissionsResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, err
	}

	return &repoUserPermissionsResponse, nil
}

// UpdateRepoUserPermission updates user permission of specific user under provided repository.
func (c *Client) UpdateRepoUserPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	userId string,
	permission string,
) error {
	encodedWorkspaceId, encodedUserId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(userId), url.PathEscape(repoId)
	urlAddress, err := url.Parse(fmt.Sprintf(RepoUserPermissionBaseURL, encodedWorkspaceId, encodedRepoId, encodedUserId))
	if err != nil {
		return err
	}

	err = c.put(
		ctx,
		urlAddress,
		UpdatePermissionPayload{
			Permission: permission,
		},
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// DeleteRepoUserPermission removes user permission of specific user under provided repository.
func (c *Client) DeleteRepoUserPermission(
	ctx context.Context,
	workspaceId string,
	repoId string,
	userId string,
) error {
	encodedWorkspaceId, encodedUserId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(userId), url.PathEscape(repoId)
	url, err := url.Parse(fmt.Sprintf(RepoUserPermissionBaseURL, encodedWorkspaceId, encodedRepoId, encodedUserId))
	if err != nil {
		return err
	}

	err = c.delete(ctx, url)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) delete(ctx context.Context, urlAddress *url.URL) error {
	req, err := c.createRequest(ctx, urlAddress, http.MethodDelete, nil, nil)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(req, uhttp.WithErrorResponse(&errRes))
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) get(ctx context.Context, urlAddress *url.URL, resourceResponse interface{}, paramOptions []QueryParam) error {
	req, err := c.createRequest(ctx, urlAddress, http.MethodGet, nil, paramOptions)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(req, uhttp.WithErrorResponse(&errRes), uhttp.WithJSONResponse(resourceResponse))
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) put(ctx context.Context, urlAddress *url.URL, data, resourceResponse interface{}, paramOptions []QueryParam) error {
	req, err := c.createRequest(ctx, urlAddress, http.MethodPut, data, paramOptions)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(req, uhttp.WithErrorResponse(&errRes), uhttp.WithJSONResponse(resourceResponse))
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) createRequest(
	ctx context.Context,
	urlAddress *url.URL,
	method string,
	data interface{},
	paramOptions []QueryParam,
) (*http.Request, error) {
	opts := []uhttp.RequestOption{
		uhttp.WithAcceptJSONHeader(),
	}
	if data != nil {
		opts = append(opts, uhttp.WithJSONBody(data))
	}

	req, err := c.wrapper.NewRequest(
		ctx,
		method,
		urlAddress,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	if paramOptions != nil {
		queryParams := url.Values{}
		for _, q := range paramOptions {
			q.setup(&queryParams)
		}

		req.URL.RawQuery = queryParams.Encode()
	}

	return req, nil
}

func handlePagination[T any](resp ListResponse[T]) ([]T, string, error) {
	if resp.PaginationData.Next != "" {
		return resp.Values, parsePageFromURL(resp.PaginationData.Next), nil
	}

	return resp.Values, "", nil
}

func mapUsers(members []WorkspaceMember) []User {
	var users []User

	for _, member := range members {
		users = append(users, member.User)
	}

	return users
}

func parsePageFromURL(urlPayload string) string {
	if urlPayload == "" {
		return ""
	}

	u, err := url.Parse(urlPayload)
	if err != nil {
		return ""
	}

	return u.Query().Get("page")
}
