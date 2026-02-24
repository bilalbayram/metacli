package schema

import (
	"context"

	"github.com/spf13/cobra"
)

type Plugin interface {
	Name() string
	Domain() string
	Register(root *cobra.Command)
}

type SchemaProvider interface {
	GetPack(domain string, version string) (*Pack, error)
	ListPacks() ([]PackRef, error)
	Sync(ctx context.Context, channel string) ([]PackRef, error)
	SyncWithRequest(ctx context.Context, request SyncRequest) (SyncResult, error)
}
