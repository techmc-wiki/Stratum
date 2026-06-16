package cli

import (
	"context"

	"github.com/stratummc/stratum/internal/resourcepolicy"
	"github.com/stratummc/stratum/internal/storage/filesystem"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

func ensureResourcePolicy(ctx context.Context, store *filesystem.Store) (resourcepolicy.Policy, error) {
	value, err := store.GetResourcePolicy(ctx, "default")
	if err == nil {
		return value, nil
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return resourcepolicy.Policy{}, err
	}
	value = resourcepolicy.MVPDefault()
	if err := store.CreateResourcePolicy(ctx, value); err != nil {
		return resourcepolicy.Policy{}, err
	}
	return value, nil
}
