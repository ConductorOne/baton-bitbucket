package connector

import (
	"context"

	"github.com/ConductorOne/baton-bitbucket/common"
	"github.com/ConductorOne/baton-bitbucket/pkg/bitbucket"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
)

var (
	resourceTypeWorkspace = &v2.ResourceType{
		Id:          "workspace",
		DisplayName: "Workspace",
		Traits:      []v2.ResourceType_Trait{},
	}
	resourceTypeUser = &v2.ResourceType{
		Id:          "user",
		DisplayName: "User",
		Traits: []v2.ResourceType_Trait{
			v2.ResourceType_TRAIT_USER,
		},
	}
)

type BitBucket struct {
	client *bitbucket.Client
}

func (bb *BitBucket) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	return []connectorbuilder.ResourceSyncer{
		workspaceBuilder(bb.client),
		// projectBuilder(bb.client),
		// userGroupBuilder(bb.client),
		userBuilder(bb.client),
		// are these neccessary?
		// roleBuilder(bb.client),
	}
}

// Metadata returns metadata about the connector.
func (bb *BitBucket) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "BitBucket",
	}, nil
}

// Validate hits the BitBucket API to validate that the configured credentials are valid and compatible.
func (bb *BitBucket) Validate(ctx context.Context) (annotations.Annotations, error) {
	// TODO: add validation
	return nil, nil
}

func New(ctx context.Context, workspaces []string, auth common.AuthOption) (*BitBucket, error) {
	httpClient, err := uhttp.NewClient(ctx, uhttp.WithLogger(true, ctxzap.Extract(ctx)))

	if err != nil {
		return nil, err
	}

	return &BitBucket{
		client: bitbucket.NewClient(auth(), httpClient),
	}, nil
}
