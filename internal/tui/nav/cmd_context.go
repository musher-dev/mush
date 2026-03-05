package nav

import (
	"context"
	"time"
)

const (
	// navStatusTimeout bounds status/health/version checks to keep the UI responsive.
	navStatusTimeout = 5 * time.Second
)

func navBaseCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

func navStatusCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(navBaseCtx(ctx), navStatusTimeout)
}
