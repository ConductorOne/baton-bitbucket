package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

	ProjectPermissionsBaseURL      = WorkspacesBaseURL + "/%s/projects/%s/permissions-config"
	ProjectGroupPermissionsBaseURL = ProjectPermissionsBaseURL + "/groups"
	ProjectUserPermissionsBaseURL  = ProjectPermissionsBaseURL + "/users"

	RepoPermissionsBaseURL      = ProjectRepositoriesBaseURL + "/%s/permissions-config"
	RepoGroupPermissionsBaseURL = RepoPermissionsBaseURL + "/groups"
	RepoUserPermissionsBaseURL  = RepoPermissionsBaseURL + "/users"
)

var defaultFilters = []string{
	"-links",
	"-*.links",
	"-*.*.links",
}

type Client struct {
	httpClient *http.Client
	auth       string
	scope      Scope
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type WorkspacesResponse struct {
	Values []Workspace `json:"values"`
	PaginationData
}

type WorkspaceResponse = Workspace

type WorkspaceMembersResponse struct {
	Values []WorkspaceMember `json:"values"`
	PaginationData
}

type WorkspaceUserGroupsResponse = []UserGroup
type UserGroupMembersResponse = []User

type UserResponse = User

type WorkspaceProjectsResponse struct {
	Values []Project `json:"values"`
	PaginationData
}

type ProjectRepositoriesResponse struct {
	Values []Repository `json:"values"`
	PaginationData
}

type GroupPermissionsResponse struct {
	Values []GroupPermission `json:"values"`
	PaginationData
}

type UserPermissionsResponse struct {
	Values []UserPermission `json:"values"`
	PaginationData
}

type PaginationVars struct {
	Limit int
	Page  string
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

func setupPaginationQuery(query *url.Values, limit int, page string) {
	// add limit
	if limit != 0 {
		query.Set("pagelen", strconv.Itoa(limit))
	}

	// add page
	if page != "" {
		query.Set("page", page)
	}
}

func setupQuery(query *url.Values, searchId string, filters ...string) {
	if searchId != "" {
		query.Set("q", searchId)
	}

	// add filters to minimize response size
	if len(filters) > 0 {
		query.Set("fields", strings.Join(filters, ","))
	}
}

// GetWorkspaces lists all workspaces current user belongs to.
func (c *Client) GetWorkspaces(ctx context.Context, getWorkspacesVars PaginationVars) ([]Workspace, string, error) {
	queryParams := url.Values{}
	setupQuery(&queryParams, "", defaultFilters...)
	setupPaginationQuery(&queryParams, getWorkspacesVars.Limit, getWorkspacesVars.Page)

	var workspacesResponse WorkspacesResponse
	err := c.doRequest(
		ctx,
		WorkspacesBaseURL,
		&workspacesResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if workspacesResponse.Next != "" {
		return workspacesResponse.Values, parsePageFromURL(workspacesResponse.Next), nil
	}

	return workspacesResponse.Values, "", nil
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
	queryParams := url.Values{}
	setupQuery(&queryParams, "", defaultFilters...)

	var workspaceResponse WorkspaceResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(WorkspaceBaseURL, encodedWorkspaceId),
		&workspaceResponse,
		queryParams,
	)

	if err != nil {
		return nil, err
	}

	return &workspaceResponse, nil
}

// GetWorkspaceMembers lists all users that belong under specified workspace.
func (c *Client) GetWorkspaceMembers(ctx context.Context, workspaceId string, getWorkspacesVars PaginationVars) ([]User, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	queryParams := url.Values{}
	setupQuery(&queryParams, "", composeFilters(defaultFilters, "-*.workspace")...)
	setupPaginationQuery(&queryParams, getWorkspacesVars.Limit, getWorkspacesVars.Page)

	var workspaceMembersResponse WorkspaceMembersResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(WorkspaceMembersBaseURL, encodedWorkspaceId),
		&workspaceMembersResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	results := mapUsers(workspaceMembersResponse.Values)

	if workspaceMembersResponse.Next != "" {
		return results, parsePageFromURL(workspaceMembersResponse.Next), nil
	}

	return results, "", nil
}

// GetWorkspaceUserGroups lists all user groups that belong under specified workspace (This method is supported only for v1 API).
func (c *Client) GetWorkspaceUserGroups(ctx context.Context, workspaceId string) ([]UserGroup, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)

	var workspaceUserGroupsResponse WorkspaceUserGroupsResponse
	err := c.doRequest(
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

	var userGroupMembersResponse UserGroupMembersResponse
	err := c.doRequest(
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

// GetCurrentUser get information about currently logged in user or team.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	queryParams := url.Values{}
	setupQuery(&queryParams, "", defaultFilters...)

	var userResponse UserResponse
	err := c.doRequest(
		ctx,
		CurrentUserBaseURL,
		&userResponse,
		queryParams,
	)

	if err != nil {
		return nil, err
	}

	return &userResponse, nil
}

// GetUser get detail information about specified user.
func (c *Client) GetUser(ctx context.Context, userId string) (*User, error) {
	encodedUserId := url.PathEscape(userId)
	queryParams := url.Values{}
	setupQuery(&queryParams, "", defaultFilters...)

	var userResponse UserResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(UserBaseURL, encodedUserId),
		&userResponse,
		queryParams,
	)

	if err != nil {
		return nil, err
	}

	return &userResponse, nil
}

// GetWorkspaceProjects lists all projects that belong under specified workspace.
func (c *Client) GetWorkspaceProjects(ctx context.Context, workspaceId string, getWorkspaceProjectsVars PaginationVars) ([]Project, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	queryParams := url.Values{}
	setupQuery(&queryParams, "", composeFilters(defaultFilters, "-*.workspace", "-*.owner")...)
	setupPaginationQuery(&queryParams, getWorkspaceProjectsVars.Limit, getWorkspaceProjectsVars.Page)

	var workspaceProjectsResponse WorkspaceProjectsResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(WorkspaceProjectsBaseURL, encodedWorkspaceId),
		&workspaceProjectsResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if workspaceProjectsResponse.Next != "" {
		return workspaceProjectsResponse.Values, parsePageFromURL(workspaceProjectsResponse.Next), nil
	}

	return workspaceProjectsResponse.Values, "", nil
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
	queryParams := url.Values{}
	setupQuery(
		&queryParams,
		fmt.Sprintf("project.uuid=\"%s\"", projectId),
		composeFilters(defaultFilters, "-*.workspace", "-*.owner")...,
	)
	setupPaginationQuery(&queryParams, getProjectReposVars.Limit, getProjectReposVars.Page)

	var projectRepositoriesResponse ProjectRepositoriesResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(ProjectRepositoriesBaseURL, encodedWorkspaceId),
		&projectRepositoriesResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if projectRepositoriesResponse.Next != "" {
		return projectRepositoriesResponse.Values, parsePageFromURL(projectRepositoriesResponse.Next), nil
	}

	return projectRepositoriesResponse.Values, "", nil
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
	queryParams := url.Values{}
	setupQuery(&queryParams, "", composeFilters(defaultFilters, "-*.*.workspace", "-*.*.owner")...)
	setupPaginationQuery(&queryParams, getPermissionsVars.Limit, getPermissionsVars.Page)
	encodedWorkspaceId := url.PathEscape(workspaceId)

	var projectGroupPermissionsResponse GroupPermissionsResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(ProjectGroupPermissionsBaseURL, encodedWorkspaceId, projectKey),
		&projectGroupPermissionsResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if projectGroupPermissionsResponse.Next != "" {
		return projectGroupPermissionsResponse.Values, parsePageFromURL(projectGroupPermissionsResponse.Next), nil
	}

	return projectGroupPermissionsResponse.Values, "", nil
}

// GetProjectUserPermissions lists all user permissions that belong under specified project.
func (c *Client) GetProjectUserPermissions(ctx context.Context, workspaceId string, projectKey string, getPermissionsVars PaginationVars) ([]UserPermission, string, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)
	queryParams := url.Values{}
	setupQuery(&queryParams, "", defaultFilters...)
	setupPaginationQuery(&queryParams, getPermissionsVars.Limit, getPermissionsVars.Page)

	var projectUserPermissionsResponse UserPermissionsResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(ProjectUserPermissionsBaseURL, encodedWorkspaceId, projectKey),
		&projectUserPermissionsResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if projectUserPermissionsResponse.Next != "" {
		return projectUserPermissionsResponse.Values, parsePageFromURL(projectUserPermissionsResponse.Next), nil
	}

	return projectUserPermissionsResponse.Values, "", nil
}

// GetRepositoryGroupPermissions lists all group permissions that belong under specified repository.
func (c *Client) GetRepositoryGroupPermissions(ctx context.Context, workspaceId string, repoId string, getPermissionsVars PaginationVars) ([]GroupPermission, string, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	queryParams := url.Values{}
	setupQuery(&queryParams, "", composeFilters(defaultFilters, "-*.*.workspace", "-*.*.owner")...)
	setupPaginationQuery(&queryParams, getPermissionsVars.Limit, getPermissionsVars.Page)

	var repositoryGroupPermissionsResponse GroupPermissionsResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(RepoGroupPermissionsBaseURL, encodedWorkspaceId, encodedRepoId),
		&repositoryGroupPermissionsResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if repositoryGroupPermissionsResponse.Next != "" {
		return repositoryGroupPermissionsResponse.Values, parsePageFromURL(repositoryGroupPermissionsResponse.Next), nil
	}

	return repositoryGroupPermissionsResponse.Values, "", nil
}

// GetRepositoryUserPermissions lists all user permissions that belong under specified repository.
func (c *Client) GetRepositoryUserPermissions(ctx context.Context, workspaceId string, repoId string, getPermissionsVars PaginationVars) ([]UserPermission, string, error) {
	encodedWorkspaceId, encodedRepoId := url.PathEscape(workspaceId), url.PathEscape(repoId)
	queryParams := url.Values{}
	setupQuery(&queryParams, "", defaultFilters...)
	setupPaginationQuery(&queryParams, getPermissionsVars.Limit, getPermissionsVars.Page)

	var repositoryUserPermissionsResponse UserPermissionsResponse
	err := c.doRequest(
		ctx,
		fmt.Sprintf(RepoUserPermissionsBaseURL, encodedWorkspaceId, encodedRepoId),
		&repositoryUserPermissionsResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", err
	}

	if repositoryUserPermissionsResponse.Next != "" {
		return repositoryUserPermissionsResponse.Values, parsePageFromURL(repositoryUserPermissionsResponse.Next), nil
	}

	return repositoryUserPermissionsResponse.Values, "", nil
}

func (c *Client) doRequest(ctx context.Context, url string, resourceResponse interface{}, queryParams url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if queryParams != nil {
		req.URL.RawQuery = queryParams.Encode()
	}

	req.Header.Set("Authorization", c.auth)
	req.Header.Set("accept", "application/json")

	rawResponse, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer rawResponse.Body.Close()

	if rawResponse.StatusCode >= 300 {
		return status.Error(codes.Code(rawResponse.StatusCode), "Request failed")
	}

	if err := json.NewDecoder(rawResponse.Body).Decode(&resourceResponse); err != nil {
		return err
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

func composeFilters(filters []string, newFilters ...string) []string {
	return append(filters, newFilters...)
}
