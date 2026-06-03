package logic

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"richcode.cc/dex/limitorder/internal/chain"
	"richcode.cc/dex/limitorder/internal/config"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"
	"richcode.cc/dex/model/limitordermodel"
)

// TestSyncMarginAccount 测试同步保证金账户数据
//
// 使用方法：
// go test -v -run TestSyncMarginAccount ./internal/logic/
func TestSyncMarginAccount(t *testing.T) {
	// 加载配置
	var c config.Config
	conf.MustLoad("../../etc/limitorder.yaml", &c)

	// 初始化数据库连接
	db, err := gorm.Open(mysql.Open(c.DSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// 初始化 Solana 客户端
	var solanaClient *chain.SolanaClient
	if c.Sol.Enable && len(c.Sol.NodeUrl) > 0 {
		solanaClient, err = chain.NewSolanaClient(c.Sol.NodeUrl[0])
		if err != nil {
			t.Fatalf("Failed to initialize Solana client: %v", err)
		}
	}

	// 创建服务上下文
	svcCtx := &svc.ServiceContext{
		Config:                c,
		DB:                    db,
		LimitOrderConfigModel: limitordermodel.NewLimitOrderConfigModel(db),
		LimitOrderMarketModel: limitordermodel.NewLimitOrderMarketModel(db),
		LimitOrderModel:       limitordermodel.NewLimitOrderModel(db),
		LimitOrderFillModel:   limitordermodel.NewLimitOrderFillModel(db),
		LimitOrderMarginModel: limitordermodel.NewLimitOrderMarginModel(db),
		SolanaClient:          solanaClient,
	}

	// 测试用例
	testCases := []struct {
		name              string
		userWalletAddress string
		marketPda         string
		wantErr           bool
	}{
		{
			name:              "已存在的 margin 账户",
			userWalletAddress: "4XVVt8NTcK7aV3yLcCMTGvdutuvZzvntUwTTjBdtoZkZ", // 替换为你的测试地址
			marketPda:         "7iNHNjRxNAm27mJySgJPmgda1hycRV7o2jUr5d8MGGbK", // 替换为你的市场 PDA
			wantErr:           false,
		},
		// 可以添加更多测试用例
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建 logic
			logic := NewSyncMarginAccountLogic(ctx, svcCtx)

			// 调用同步方法
			resp, err := logic.SyncMarginAccount(&limitorder.SyncMarginAccountRequest{
				ChainId:           c.Sol.ChainId,
				UserWalletAddress: tc.userWalletAddress,
				MarketPda:         tc.marketPda,
			})

			// 检查错误
			if (err != nil) != tc.wantErr {
				t.Errorf("SyncMarginAccount() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if err != nil {
				t.Logf("Expected error: %v", err)
				return
			}

			// 打印结果
			t.Logf("✅ Sync successful!")
			t.Logf("Message: %s", resp.Message)
			if resp.Margin != nil {
				t.Logf("\n📊 Margin Account Data:")
				t.Logf("  ID:            %d", resp.Margin.Id)
				t.Logf("  Margin PDA:    %s", resp.Margin.MarginPda)
				t.Logf("  Market PDA:    %s", resp.Margin.MarketPda)
				t.Logf("  Owner:         %s", resp.Margin.Owner)
				t.Logf("  Base Free:     %s", resp.Margin.BaseFree)
				t.Logf("  Base Locked:   %s", resp.Margin.BaseLocked)
				t.Logf("  Quote Free:    %s", resp.Margin.QuoteFree)
				t.Logf("  Quote Locked:  %s", resp.Margin.QuoteLocked)
				t.Logf("  Last Sync Slot: %d", resp.Margin.LastSyncSlot)
			}
		})
	}
}

// TestSyncMultipleMarginAccounts 测试同步多个保证金账户
//
// 使用方法：
// go test -v -run TestSyncMultipleMarginAccounts ./limitorder/internal/logic/
func TestSyncMultipleMarginAccounts(t *testing.T) {
	// 加载配置
	var c config.Config
	conf.MustLoad("../../etc/limitorder.yaml", &c)

	// 初始化数据库连接
	db, err := gorm.Open(mysql.Open(c.DSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// 初始化 Solana 客户端
	var solanaClient *chain.SolanaClient
	if c.Sol.Enable && len(c.Sol.NodeUrl) > 0 {
		solanaClient, err = chain.NewSolanaClient(c.Sol.NodeUrl[0])
		if err != nil {
			t.Fatalf("Failed to initialize Solana client: %v", err)
		}
	}

	// 创建服务上下文
	svcCtx := &svc.ServiceContext{
		Config:                c,
		DB:                    db,
		LimitOrderConfigModel: limitordermodel.NewLimitOrderConfigModel(db),
		LimitOrderMarketModel: limitordermodel.NewLimitOrderMarketModel(db),
		LimitOrderModel:       limitordermodel.NewLimitOrderModel(db),
		LimitOrderFillModel:   limitordermodel.NewLimitOrderFillModel(db),
		LimitOrderMarginModel: limitordermodel.NewLimitOrderMarginModel(db),
		SolanaClient:          solanaClient,
	}

	// 定义要同步的用户地址列表
	userAddresses := []string{
		"6QC9MjYEBQ8XXxgs4ipJbJ3LFSK4zNvQgqg2SyaWYHW5", // 替换为你的测试地址
		// 可以添加更多地址
	}

	// 定义要同步的市场 PDA
	marketPda := "7iNHNjRxNAm27mJySgJPmgda1hycRV7o2jUr5d8MGGbK" // 替换为你的市场 PDA

	ctx := context.Background()
	logic := NewSyncMarginAccountLogic(ctx, svcCtx)

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("同步多个 Margin Account 到数据库")
	fmt.Println(strings.Repeat("=", 80))

	successCount := 0
	failCount := 0

	for i, userAddr := range userAddresses {
		fmt.Printf("\n[%d/%d] 同步用户: %s\n", i+1, len(userAddresses), userAddr)
		fmt.Println(strings.Repeat("-", 80))

		resp, err := logic.SyncMarginAccount(&limitorder.SyncMarginAccountRequest{
			ChainId:           c.Sol.ChainId,
			UserWalletAddress: userAddr,
			MarketPda:         marketPda,
		})

		if err != nil {
			fmt.Printf("❌ 同步失败: %v\n", err)
			failCount++
			continue
		}

		fmt.Printf("✅ 同步成功: %s\n", resp.Message)
		if resp.Margin != nil {
			fmt.Printf("   Margin PDA: %s\n", resp.Margin.MarginPda)
			fmt.Printf("   Base Free: %s | Base Locked: %s\n",
				resp.Margin.BaseFree, resp.Margin.BaseLocked)
			fmt.Printf("   Quote Free: %s | Quote Locked: %s\n",
				resp.Margin.QuoteFree, resp.Margin.QuoteLocked)
		}
		successCount++
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("同步完成: 成功 %d, 失败 %d\n", successCount, failCount)
	fmt.Println(strings.Repeat("=", 80))

	if failCount > 0 {
		t.Errorf("有 %d 个账户同步失败", failCount)
	}
}

// TestQueryMarginAccount 测试查询已存在的保证金账户（不写入数据库）
//
// 使用方法：
// go test -v -run TestQueryMarginAccount ./limitorder/internal/logic/
func TestQueryMarginAccount(t *testing.T) {
	// 加载配置
	var c config.Config
	conf.MustLoad("../../etc/limitorder.yaml", &c)

	// 初始化 Solana 客户端
	var solanaClient *chain.SolanaClient
	var err error
	if c.Sol.Enable && len(c.Sol.NodeUrl) > 0 {
		solanaClient, err = chain.NewSolanaClient(c.Sol.NodeUrl[0])
		if err != nil {
			t.Fatalf("Failed to initialize Solana client: %v", err)
		}
	}

	// 测试参数（替换为你的测试数据）
	userWalletAddress := "6QC9MjYEBQ8XXxgs4ipJbJ3LFSK4zNvQgqg2SyaWYHW5"
	marketPda := "7iNHNjRxNAm27mJySgJPmgda1hycRV7o2jUr5d8MGGbK"

	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("查询 Margin Account 数据\n")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("用户地址: %s\n", userWalletAddress)
	fmt.Printf("市场 PDA: %s\n", marketPda)
	fmt.Println()

	// 解析地址
	marketPubkey, err := solana.PublicKeyFromBase58(marketPda)
	if err != nil {
		t.Fatalf("Invalid market PDA: %v", err)
	}

	ownerPubkey, err := solana.PublicKeyFromBase58(userWalletAddress)
	if err != nil {
		t.Fatalf("Invalid user wallet address: %v", err)
	}

	// 计算 margin PDA
	marginPDA, bump, err := solanaClient.FindMarginPDA(marketPubkey, ownerPubkey)
	if err != nil {
		t.Fatalf("Failed to find margin PDA: %v", err)
	}

	fmt.Printf("计算的 Margin PDA: %s (bump: %d)\n\n", marginPDA.String(), bump)

	// 查询账户信息
	ctx := context.Background()
	accountInfo, err := solanaClient.GetAccountInfo(ctx, marginPDA)
	if err != nil {
		t.Fatalf("Failed to get account info: %v", err)
	}

	if accountInfo.Value == nil {
		t.Fatal("Margin account does not exist")
	}

	data := accountInfo.Value.Data.GetBinary()
	fmt.Printf("账户数据长度: %d bytes\n", len(data))
	fmt.Printf("账户所有者: %s\n", accountInfo.Value.Owner.String())
	fmt.Printf("当前 Slot: %d\n\n", accountInfo.Context.Slot)

	// 反序列化数据
	if len(data) < 8+32+32+8+8+8+8+1 {
		t.Fatalf("Invalid account data length")
	}

	offset := 8
	owner := solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32
	market := solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32
	baseFree := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	quoteFree := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	baseLocked := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	quoteLocked := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	bumpFromAccount := data[offset]

	fmt.Println("📊 Margin Account 数据:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Owner:        %s\n", owner.String())
	fmt.Printf("Market:       %s\n", market.String())
	fmt.Printf("Base Free:    %d\n", baseFree)
	fmt.Printf("Quote Free:   %d\n", quoteFree)
	fmt.Printf("Base Locked:  %d\n", baseLocked)
	fmt.Printf("Quote Locked: %d\n", quoteLocked)
	fmt.Printf("Bump:         %d\n", bumpFromAccount)
	fmt.Println(strings.Repeat("=", 80))
}
