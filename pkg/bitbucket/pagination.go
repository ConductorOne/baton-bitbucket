package bitbucket

import "net/url"

func ParsePageFromURL(urlPayload string) string {
	if urlPayload == "" {
		return ""
	}

	u, err := url.Parse(urlPayload)
	if err != nil {
		return ""
	}

	return u.Query().Get("page")
}

func HandlePagination[T any](response ListResponse[T], err error) ([]T, string, error) {
	if err != nil {
		return nil, "", err
	}

	nextToken := ""
	if response.PaginationData.Next != "" {
		nextToken = ParsePageFromURL(response.PaginationData.Next)
	}

	return response.Values, nextToken, nil
}
