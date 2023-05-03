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

const BaseURL = "https://api.bitbucket.org/2.0/"
const WorkspacesBaseURL = BaseURL + "workspaces"
const WorkspaceMembersBaseURL = WorkspacesBaseURL + "/%s/members"

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

// GetWorkspaces lists all workspaces current user belongs to.
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
