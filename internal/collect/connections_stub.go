//go:build !darwin

package collect

import (
	"context"

	"github.com/daniel/sysdash/internal/domain"
)

func ProcessConnections(context.Context) ([]domain.ConnectionInfo, error) {
	return nil, nil
}
