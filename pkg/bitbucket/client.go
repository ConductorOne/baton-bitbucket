package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
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

// WorkspaceId If client have access only to one workspace, method `WorkspaceId`
// returns that id otherwise it returns error.
func (c *Client) WorkspaceId() (string, error) {
	if c.IsWorkspaceScoped() {
		return c.scope.(*WorkspaceScoped).Workspace, nil
	}
	return "", status.Error(codes.InvalidArgument, "client is not workspace scoped")
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

// checkPermissions given a workspace, use the API to check if we have
// permissions to get usergroups, members, and projects.
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

// filterWorkspaces given a list of workspaces, filter down to a list of
// workspaces that have been explicitly selected in configs.
func (c *Client) filterWorkspaces(ctx context.Context, workspaces []Workspace) ([]Workspace, error) {
	filteredWorkspaces := make([]Workspace, 0)
	for _, workspace := range workspaces {
		// We call this function in order to initialize the workspaceID's map.
		// In that case we need to return all workspaces, so they can be
		// filtered and only the valid ones are set in the workspaceIds map.
		if shouldIncludeWorkspace(c.workspaceIDs, workspace.Id) {
			filteredWorkspaces = append(filteredWorkspaces, workspace)
		}
	}

	return filteredWorkspaces, nil
}

// shouldIncludeWorkspace given a set of `workspaceIdentifiers` (i.e. a set of
// slugs or a set of IDs), determine whether an workspaceIdentifier is in the
// set.
func shouldIncludeWorkspace(
	workspaceIdentifiers mapset.Set[string],
	workspaceIdentifier string,
) bool {
	if workspaceIdentifiers.Cardinality() == 0 {
		// If none are selected, then we want _all_ workspaces.
		return true
	}
	return workspaceIdentifiers.Contains(workspaceIdentifier)
}

// SetWorkspaceIDs If client has access to multiple workspaces, method
// WorkspaceIDs` sets `c.workspaceIDs` to a list of workspace ids. Otherwise,
// it returns error.
func (c *Client) SetWorkspaceIDs(ctx context.Context, workspaceSlugs []string) error {
	wantedWorkspaceSlugs := mapset.NewSet(workspaceSlugs...)

	if !c.IsUserScoped() {
		return status.Error(codes.InvalidArgument, "client is not user scoped")
	}

	workspaces, err := c.GetAllWorkspaces(ctx)
	if err != nil {
		return err
	}

	for _, workspace := range workspaces {
		if shouldIncludeWorkspace(wantedWorkspaceSlugs, workspace.Slug) {
			hasPermission, err := c.checkPermissions(ctx, &workspace)
			if err != nil {
				return err
			}
			if hasPermission {
				c.workspaceIDs.Add(workspace.Id)
			}
		}
	}

	if c.workspaceIDs.Cardinality() == 0 {
		return status.Error(codes.Unauthenticated, "no authenticated workspaces found")
	}
	return nil
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
	return handlePagination(workspacesResponse, err)
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

func (c *Client) getUrl(path string, args ...string) *url.URL {
	escapedArgs := make([]string, 0)
	for _, arg := range args {
		escapedArgs = append(escapedArgs, url.PathEscape(arg))
	}
	return c.baseUrl.JoinPath(fmt.Sprintf(path, escapedArgs))
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

	members, page, _ := handlePagination(workspaceMembersResponse, nil)

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
	return handlePagination(workspaceProjectsResponse, err)
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
	return handlePagination(projectRepositoriesResponse, err)
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
	return handlePagination(projectGroupPermissionsResponse, err)
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
	return handlePagination(projectUserPermissionsResponse, err)
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

	return handlePagination(repositoryGroupPermissionsResponse, err)
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
	return handlePagination(repositoryUserPermissionsResponse, err)
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

func (c *Client) delete(ctx context.Context, url *url.URL) error {
	req, err := c.createRequest(ctx, url, http.MethodDelete, nil, nil)
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

func (c *Client) get(
	ctx context.Context,
	url *url.URL,
	resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	req, err := c.createRequest(ctx, url, http.MethodGet, nil, paramOptions)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(
		req,
		uhttp.WithErrorResponse(&errRes),
		uhttp.WithJSONResponse(resourceResponse),
	)
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) put(ctx context.Context, url *url.URL, data, resourceResponse interface{}, paramOptions []QueryParam) error {
	request, err := c.createRequest(ctx, url, http.MethodPut, data, paramOptions)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(
		request,
		uhttp.WithErrorResponse(&errRes),
		uhttp.WithJSONResponse(resourceResponse),
	)
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) createRequest(
	ctx context.Context,
	url0 *url.URL,
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

	request, err := c.wrapper.NewRequest(
		ctx,
		method,
		url0,
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

		request.URL.RawQuery = queryParams.Encode()
	}

	return request, nil
}

func handlePagination[T any](response ListResponse[T], err error) ([]T, string, error) {
	if err != nil {
		return nil, "", err
	}

	nextToken := ""
	if response.PaginationData.Next != "" {
		nextToken = parsePageFromURL(response.PaginationData.Next)
	}

	return response.Values, nextToken, nil
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
