package bitbucket

import "fmt"

type Scope interface {
	String() string
	WorkspaceId() string
}

type UserScoped struct {
	Username string
}

func (u *UserScoped) String() string {
	return fmt.Sprintf("user:%s", u.Username)
}

func (u *UserScoped) WorkspaceId() string {
	return ""
}

type WorkspaceScoped struct {
	Workspace string
}

func (w *WorkspaceScoped) String() string {
	return fmt.Sprintf("workspace:%s", w.Workspace)
}

func (w *WorkspaceScoped) WorkspaceId() string {
	return w.Workspace
}

type ProjectScoped struct {
	Workspace string
	Project   string
}

func (p *ProjectScoped) String() string {
	return fmt.Sprintf("project:%s:%s", p.Workspace, p.Project)
}

func (p *ProjectScoped) WorkspaceId() string {
	return p.Workspace
}

type RepositoryScoped struct {
	Workspace  string
	Project    string
	Repository string
}

func (r *RepositoryScoped) String() string {
	return fmt.Sprintf("repository:%s:%s:%s", r.Workspace, r.Project, r.Repository)
}

func (r *RepositoryScoped) WorkspaceId() string {
	return r.Workspace
}
