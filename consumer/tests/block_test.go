package tests

import (
	"context"
	"os"
	"testing"

	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/consumer/internal/logic/block"
	"richcode.cc/dex/consumer/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestBlock(t *testing.T) {

	var slot int64 = 464087069

	cfgFile := os.Getenv("CONSUMER_CONFIG")
	if cfgFile == "" {
		cfgFile = "../etc/consumer.yaml"
	}

	var cfg config.Config
	if err := conf.Load(cfgFile, &cfg); err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	config.SaveConf(cfg)

	ctx := svc.NewSolServiceContext(cfg)

	// slotChan is only needed to satisfy the constructor; ProcessBlock is invoked directly.
	slotChan := make(chan uint64, 1)
	bs := block.NewBlockService(ctx, "block-test", slotChan, 0)

	bs.ProcessBlock(context.Background(), slot)
}
