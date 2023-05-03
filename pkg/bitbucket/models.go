package bitbucket

type BaseResource struct {
	Id string `json:"uuid"`
}

type Workspace struct {
	BaseResource
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type WorkspaceMember struct {
	User      User      `json:"user"`
	Workspace Workspace `json:"workspace"`
}

type User struct {
	BaseResource
	Name     string `json:"display_name"`
	Nickname string `json:"nickname"`
}

type PaginationData struct {
	Next string `json:"next"`
}
