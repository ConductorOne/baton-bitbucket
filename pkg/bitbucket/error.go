package bitbucket

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (er *errorResponse) Message() string {
	return fmt.Sprintf("Error: %s", er.Error.Message)
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
