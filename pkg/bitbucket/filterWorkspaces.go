package bitbucket

import (
	"context"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
