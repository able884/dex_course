package rds

import "context"

func RPushWithExpired(ctx context.Context, key string, seconds int, list ...any) (int, error) {
	count, err := Client.RpushCtx(ctx, key, list...)
	err = Client.ExpireCtx(ctx, key, seconds)
	if err != nil {
		return 0, err
	}
	return count, err
}

func LPushWithExpired(ctx context.Context, key string, seconds int, list ...any) (int, error) {
	count, err := Client.LpushCtx(ctx, key, list...)
	return count, err
}
