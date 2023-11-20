package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	V1BaseURL = "https://api.bitbucket.org/1.0/"
	BaseURL   = "https://api.bitbucket.org/2.0/"

	LoginBaseURL = "https://bitbucket.org/site/oauth2/access_token"

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
	httpClient *http.Client
	auth       string
	scope      Scope
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

func NewClient(auth string, httpClient *http.Client) *Client {
	return &Client{
		auth:       auth,
		httpClient: httpClient,
	}
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

// If client have access to multiple workspaces, method `WorkspaceIds`
// returns list of workspace ids otherwise it returns error.
func (c *Client) WorkspaceIds(ctx context.Context) ([]string, error) {
	workspaceIds := make([]string, 0)

	if c.IsUserScoped() {
		workspaces, err := c.GetAllWorkspaces(ctx)
		if err != nil {
			return nil, err
		}

		for _, workspace := range workspaces {
			workspaceIds = append(workspaceIds, workspace.Id)
		}

		if len(workspaceIds) == 0 {
			return nil, status.Error(codes.NotFound, "no workspaces found")
		}
	} else {
		return nil, status.Error(codes.InvalidArgument, "client is not user scoped")
	}

	return workspaceIds, nil
}

// GetWorkspaces lists all workspaces current user belongs to.
func (c *Client) GetWorkspaces(ctx context.Context, getWorkspacesVars PaginationVars) ([]Workspace, string, error) {
	var workspacesResponse ListResponse[Workspace]
	err := c.get(
		ctx,
		WorkspacesBaseURL,
		&workspacesResponse,
		[]QueryParam{
			&getWorkspacesVars,
			prepareFilters(""),
		},
	)

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

	var workspaceResponse Workspace
	err := c.get(
		ctx,
		fmt.Sprintf(WorkspaceBaseURL, encodedWorkspaceId),
		&workspaceResponse,
		[]QueryParam{
			prepareFilters(""),
		},
	)

	if err != nil {
		return nil, err
	}

	return &workspaceResponse, nil
}

// GetWorkspaceMembers lists all users that belong under specified workspace.
func (c *Client) GetWorkspaceMembers(ctx context.Context, workspaceId string, getWorkspacesVars PaginationVars) ([]User, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)

	var workspaceMembersResponse ListResponse[WorkspaceMember]
	err := c.get(
		ctx,
		fmt.Sprintf(WorkspaceMembersBaseURL, encodedWorkspaceId),
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

	var workspaceUserGroupsResponse []UserGroup
	err := c.get(
		ctx,
		fmt.Sprintf(WorkspaceUserGroupsBaseURL, encodedWorkspaceId),
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

	var userGroupMembersResponse []User
	err := c.get(
		ctx,
		fmt.Sprintf(UserGroupMembersBaseURL, encodedWorkspaceId, groupSlug),
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

	err := c.put(
		ctx,
		fmt.Sprintf(GroupMemberModifyBaseURL, encodedWorkspaceId, groupSlug, encodedUserId),
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

	err := c.delete(
		ctx,
		fmt.Sprintf(GroupMemberModifyBaseURL, encodedWorkspaceId, groupSlug, encodedUserId),
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// GetCurrentUser get information about currently logged in user or team.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	var userResponse User
	err := c.get(
		ctx,
		CurrentUserBaseURL,
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

	var userResponse User
	err := c.get(
		ctx,
		fmt.Sprintf(UserBaseURL, encodedUserId),
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

	var workspaceProjectsResponse ListResponse[Project]
	err := c.get(
		ctx,
		fmt.Sprintf(WorkspaceProjectsBaseURL, encodedWorkspaceId),
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

	var projectRepositoriesResponse ListResponse[Repository]
	err := c.get(
		ctx,
		fmt.Sprintf(ProjectRepositoriesBaseURL, encodedWorkspaceId),
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

	var projectGroupPermissionsResponse ListResponse[GroupPermission]
	err := c.get(
		ctx,
		fmt.Sprintf(ProjectGroupPermissionsBaseURL, encodedWorkspaceId, projectKey),
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

	var projectGroupPermissionsResponse GroupPermission
	err := c.get(
		ctx,
		fmt.Sprintf(ProjectGroupPermissionBaseURL, encodedWorkspaceId, projectKey, groupSlug),
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

	err := c.put(
		ctx,
		fmt.Sprintf(ProjectGroupPermissionBaseURL, encodedWorkspaceId, projectKey, groupSlug),
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

	err := c.delete(
		ctx,
		fmt.Sprintf(ProjectGroupPermissionBaseURL, encodedWorkspaceId, projectKey, groupSlug),
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// GetProjectUserPermissions lists all user permissions that belong under specified project.
func (c *Client) GetProjectUserPermissions(ctx context.Context, workspaceId string, projectKey string, getPermissionsVars PaginationVars) ([]UserPermission, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)

	var projectUserPermissionsResponse ListResponse[UserPermission]
	err := c.get(
		ctx,
		fmt.Sprintf(ProjectUserPermissionsBaseURL, encodedWorkspaceId, projectKey),
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

	var projectUserPermissionsResponse UserPermission
	err := c.get(
		ctx,
		fmt.Sprintf(ProjectUserPermissionBaseURL, encodedWorkspaceId, projectKey, encodedUserId),
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

	err := c.put(
		ctx,
		fmt.Sprintf(ProjectUserPermissionBaseURL, encodedWorkspaceId, projectKey, encodedUserId),
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

	err := c.delete(
		ctx,
		fmt.Sprintf(ProjectUserPermissionBaseURL, encodedWorkspaceId, projectKey, encodedUserId),
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// GetRepositoryGroupPermissions lists all group permissions that belong under specified repository.
func (c *Client) GetRepositoryGroupPermissions(ctx context.Context, workspaceId string, repoId string, getPermissionsVars PaginationVars) ([]GroupPermission, string, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)

	var repositoryGroupPermissionsResponse ListResponse[GroupPermission]
	err := c.get(
		ctx,
		fmt.Sprintf(RepoGroupPermissionsBaseURL, encodedWorkspaceId, encodedRepoId),
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

	var repoGroupPermissionsResponse GroupPermission
	err := c.get(
		ctx,
		fmt.Sprintf(RepoGroupPermissionBaseURL, encodedWorkspaceId, encodedRepoId, groupSlug),
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

	err := c.put(
		ctx,
		fmt.Sprintf(RepoGroupPermissionBaseURL, encodedWorkspaceId, encodedRepoId, groupSlug),
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

	err := c.delete(
		ctx,
		fmt.Sprintf(RepoGroupPermissionBaseURL, encodedWorkspaceId, encodedRepoId, groupSlug),
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

// GetRepositoryUserPermissions lists all user permissions that belong under specified repository.
func (c *Client) GetRepositoryUserPermissions(ctx context.Context, workspaceId string, repoId string, getPermissionsVars PaginationVars) ([]UserPermission, string, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)

	var repositoryUserPermissionsResponse ListResponse[UserPermission]
	err := c.get(
		ctx,
		fmt.Sprintf(RepoUserPermissionsBaseURL, encodedWorkspaceId, encodedRepoId),
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

	var repoUserPermissionsResponse UserPermission
	err := c.get(
		ctx,
		fmt.Sprintf(RepoUserPermissionBaseURL, encodedWorkspaceId, encodedRepoId, encodedUserId),
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

	err := c.put(
		ctx,
		fmt.Sprintf(RepoUserPermissionBaseURL, encodedWorkspaceId, encodedRepoId, encodedUserId),
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

	err := c.delete(
		ctx,
		fmt.Sprintf(RepoUserPermissionBaseURL, encodedWorkspaceId, encodedRepoId, encodedUserId),
		nil,
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

func handlePagination[T any](resp ListResponse[T]) ([]T, string, error) {
	if resp.PaginationData.Next != "" {
		return resp.Values, parsePageFromURL(resp.PaginationData.Next), nil
	}

	return resp.Values, "", nil
}

func (c *Client) get(
	ctx context.Context,
	urlAddress string,
	resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	return c.doRequest(ctx, urlAddress, http.MethodGet, nil, resourceResponse, paramOptions)
}

func (c *Client) put(
	ctx context.Context,
	urlAddress string,
	data interface{},
	resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	return c.doRequest(ctx, urlAddress, http.MethodPut, data, resourceResponse, paramOptions)
}

func (c *Client) delete(
	ctx context.Context,
	urlAddress string,
	resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	return c.doRequest(ctx, urlAddress, http.MethodDelete, nil, resourceResponse, paramOptions)
}

func (c *Client) doRequest(
	ctx context.Context,
	urlAddress string,
	method string,
	data interface{},
	resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	var body io.Reader

	if data != nil {
		jsonBody, err := json.Marshal(data)
		if err != nil {
			return err
		}

		body = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlAddress, body)
	if err != nil {
		return err
	}

	if paramOptions != nil {
		queryParams := url.Values{}
		for _, q := range paramOptions {
			q.setup(&queryParams)
		}

		req.URL.RawQuery = queryParams.Encode()
	}

	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rawResponse, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer rawResponse.Body.Close()

	if rawResponse.StatusCode >= 300 {
		return status.Error(codes.Code(rawResponse.StatusCode), "Request failed")
	}

	if method != http.MethodDelete {
		if err := json.NewDecoder(rawResponse.Body).Decode(&resourceResponse); err != nil {
			return err
		}
	}

	return nil
}

// Login exchanges the client id and secret for an access token with supported Client Credentials Grant.
func Login(client *http.Client, ctx context.Context, idAndSecret string) (string, error) {
	var loginResponse LoginResponse

	body := bytes.NewBufferString("grant_type=client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, LoginBaseURL, body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", idAndSecret)

	rawResponse, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer rawResponse.Body.Close()

	if rawResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to login: %s", rawResponse.Status)
	}

	if err := json.NewDecoder(rawResponse.Body).Decode(&loginResponse); err != nil {
		return "", err
	}

	return loginResponse.AccessToken, nil
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
