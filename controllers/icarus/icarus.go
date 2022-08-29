package icarus

import (
	"context"
	"net/http"
)

type Retry struct {
	Interval    int64  `json:"interval"`
	Strategy    string `json:"strategy"`
	MaxAttempts int64  `json:"maxAttempts"`
	Enabled     bool   `json:"enabled"`
}

type Icarus interface {
	Backup(ctx context.Context, req BackupRequest) (Backup, error)
	Backups(ctx context.Context) ([]Backup, error)
	Restore(ctx context.Context, req RestoreRequest) error
	Restores(ctx context.Context) ([]Restore, error)
}

type client struct {
	addr       string
	httpClient *http.Client
}

func New(addr string) Icarus {
	return &client{
		addr:       addr,
		httpClient: &http.Client{},
	}
}
