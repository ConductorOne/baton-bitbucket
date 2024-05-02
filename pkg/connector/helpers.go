package connector

import (
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"

	"github.com/conductorone/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var ResourcesPageSize = 50

func titleCase(s string) string {
	titleCaser := cases.Title(language.English)

	return titleCaser.String(s)
}

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

func mapUserIDs(users []bitbucket.User) []string {
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

func isUserPresent(users []bitbucket.User, targetUserId string) bool {
	for _, user := range users {
		if user.Id == targetUserId {
			return true
		}
	}

	return false
}

func splitFullName(fullName string) (string, string) {
	parts := strings.Split(fullName, " ")

	return parts[0], strings.Join(parts[1:], " ")
}

func GetIdFromComposedId(resource *v2.Resource) string {
	parts := strings.Split(resource.Id.Resource, ":")
	return parts[len(parts)-1]
}

func ParseEntitlementID(id string) (*v2.ResourceId, string, error) {
	parts := strings.Split(id, ":")

	// Need to be at least 3 parts type:entitlement_id:slug
	if len(parts) < 4 {
		return nil, "", fmt.Errorf("bitbucket-connector: invalid resource id")
	}

	resourceId := &v2.ResourceId{
		ResourceType: parts[0],
		Resource:     strings.Join(parts[1:len(parts)-1], ":"),
	}
	return resourceId, parts[len(parts)-1], nil
}
func isPermissionDeniedErr(err error) bool {
	e, ok := status.FromError(err)
	if ok && e.Code() == codes.PermissionDenied {
		return true
	}
	// In most cases the error code is unknown and the error message contains "status 403".
	if (!ok || e.Code() == codes.Unknown) && strings.Contains(err.Error(), "status 403") {
		return true
	}
	return false
}
