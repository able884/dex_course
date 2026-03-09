package block

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
)

func GetSolBlockInfoDelay(c *client.Client, ctx context.Context, slot uint64) (resp *client.Block, err error) {
	// 减少helius调用，因为失败也算次数
	time.Sleep(time.Second * 1)
	return GetSolBlockInfo(c, ctx, slot)
}

func GetSolBlockInfo(c *client.Client, ctx context.Context, slot uint64) (resp *client.Block, err error) {
	var count int64
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err = c.GetBlockWithConfig(ctx, slot, client.GetBlockConfig{
			Commitment:         rpc.CommitmentConfirmed,
			TransactionDetails: rpc.GetBlockConfigTransactionDetailsFull,
		})
		switch {
		case err == nil:
			return
		case strings.Contains(err.Error(), "Block not available for slot"):
			count++
			if count > 10 {
				return
			}
			time.Sleep(time.Second)
		case strings.Contains(err.Error(), "limit"):
			count++
			if count > 10 {
				return
			}
			time.Sleep(time.Second)
		default:
			err = fmt.Errorf("GetBlock err:%w", err)
			return
		}
	}
}
