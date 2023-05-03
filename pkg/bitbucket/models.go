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

type UserGroup struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Permission string `json:"permission"`
	Members    []User `json:"members"`
}

type Project struct {
	BaseResource
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PaginationData struct {
	Next string `json:"next"`
}
