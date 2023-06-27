package connector

import (
	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var ResourcesPageSize = 50
var titleCaser = cases.Title(language.English)

func parsePageToken(i string, resourceID *v2.ResourceId) (*pagination.Bag, error) {
	b := &pagination.Bag{}
	err := b.Unmarshal(i)
	if err != nil {
		return nil, err
	}

	if b.Current() == nil {
		b.Push(pagination.PageState{
			ResourceTypeID: resourceID.ResourceType,
			ResourceID:     resourceID.Resource,
		})
	}

	return b, nil
}

func mapUserIds(users []bitbucket.User) []string {
	ids := make([]string, len(users))

	for i, user := range users {
		ids[i] = user.Id
	}

	return ids
}

func contains(payload string, values []string) bool {
	for _, val := range values {
		if payload == val {
			return true
		}
	}

	return false
}
