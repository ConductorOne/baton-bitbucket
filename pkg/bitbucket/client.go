package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/conductorone/baton-sdk/pkg/annotations"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	V1BaseURL = "https://api.bitbucket.org/1.0/"
	BaseURL   = "https://api.bitbucket.org/2.0/"

	WorkspacesBaseURL          = BaseURL + "workspaces"
	WorkspaceMembersBaseURL    = WorkspacesBaseURL + "/%s/members"
	WorkspaceUserGroupsBaseURL = V1BaseURL + "groups/%s"
	UserBaseURL                = BaseURL + "users/%s"
)

type Client struct {
	httpClient *http.Client
	auth       string
}

type WorkspacesResponse struct {
	Results []Workspace `json:"values"`
	PaginationData
}

type WorkspaceMembersResponse struct {
	Results []WorkspaceMember `json:"values"`
	PaginationData
}

type WorkspaceUserGroupsResponse struct {
	Results []UserGroup
}

type UserResponse struct {
	Result User
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

func setupPaginationQuery(query url.Values, limit int) url.Values {
	// add limit
	if limit != 0 {
		query.Add("pagelen", strconv.Itoa(limit))
	}

	return query
}

// GetWorkspaces lists all workspaces current user belongs to.
func (c *Client) GetWorkspaces(ctx context.Context, getWorkspacesVars PaginationVars) ([]Workspace, string, annotations.Annotations, error) {
	queryParams := setupPaginationQuery(url.Values{}, getWorkspacesVars.Limit)

	var workspaceResponse WorkspacesResponse
	annos, err := c.doRequest(
		ctx,
		WorkspacesBaseURL,
		&workspaceResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", nil, err
	}

	if workspaceResponse.Next != "" {
		return workspaceResponse.Results, parsePageFromURL(workspaceResponse.Next), annos, nil
	}

	return workspaceResponse.Results, "", annos, nil
}

// GetWorkspaceMembers lists all users that belong under specified workspace.
func (c *Client) GetWorkspaceMembers(ctx context.Context, workspaceId string, getWorkspacesVars PaginationVars) ([]User, string, annotations.Annotations, error) {
	queryParams := setupPaginationQuery(url.Values{}, getWorkspacesVars.Limit)
	encodedWorkspaceId := url.PathEscape(workspaceId)

	var workspaceMembersResponse WorkspaceMembersResponse
	annos, err := c.doRequest(
		ctx,
		fmt.Sprintf(WorkspaceMembersBaseURL, encodedWorkspaceId),
		&workspaceMembersResponse,
		queryParams,
	)

	if err != nil {
		return nil, "", nil, err
	}

	results := mapUsers(workspaceMembersResponse.Results)

	if workspaceMembersResponse.Next != "" {
		return results, parsePageFromURL(workspaceMembersResponse.Next), annos, nil
	}

	return results, "", annos, nil
}

// GetWorkspaceUserGroups lists all user groups that belong under specified workspace.
func (c *Client) GetWorkspaceUserGroups(ctx context.Context, workspaceId string) ([]UserGroup, annotations.Annotations, error) {
	encodedWorkspaceId := url.PathEscape(workspaceId)

	var workspaceUserGroupsResponse WorkspaceUserGroupsResponse
	annos, err := c.doRequest(
		ctx,
		fmt.Sprintf(WorkspaceUserGroupsBaseURL, encodedWorkspaceId),
		&workspaceUserGroupsResponse,
		nil,
	)

	if err != nil {
		return nil, nil, err
	}

	return workspaceUserGroupsResponse.Results, annos, nil
}

// GetUser get detail information about specified user.
func (c *Client) GetUser(ctx context.Context, userId string) (*User, annotations.Annotations, error) {
	encodedUserId := url.PathEscape(userId)

	var userResponse UserResponse
	annos, err := c.doRequest(
		ctx,
		fmt.Sprintf(UserBaseURL, encodedUserId),
		&userResponse,
		nil,
	)

	if err != nil {
		return nil, nil, err
	}

	return &userResponse.Result, annos, nil
}

func (c *Client) doRequest(ctx context.Context, url string, resourceResponse interface{}, queryParams url.Values) (annotations.Annotations, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if queryParams != nil {
		req.URL.RawQuery = queryParams.Encode()
	}

	req.Header.Set("Authorization", fmt.Sprint(c.auth))
	req.Header.Set("accept", "application/json")

	rawResponse, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer rawResponse.Body.Close()

	if rawResponse.StatusCode >= 300 {
		return nil, status.Error(codes.Code(rawResponse.StatusCode), "Request failed")
	}

	if err := json.NewDecoder(rawResponse.Body).Decode(&resourceResponse); err != nil {
		return nil, err
	}

	annos := annotations.Annotations{}

	// TODO: add rate limits if possible

	return annos, nil
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
