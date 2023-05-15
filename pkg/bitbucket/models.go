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
	User User `json:"user"`
}

type User struct {
	BaseResource
	Type     string `json:"type"`
	Name     string `json:"display_name"`
	Username string `json:"username"`
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

type Repository struct {
	BaseResource
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
}
type Permission struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Value string `json:"permission"`
}

type GroupPermission struct {
	Permission
	Group UserGroup `json:"group"`
}

type UserPermission struct {
	Permission
	User User `json:"user"`
}

type PaginationData struct {
	Next string `json:"next"`
}
