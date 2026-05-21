package svc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"richcode.cc/dex/market/internal/config"
	"richcode.cc/dex/pkg/constants"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"richcode.cc/dex/model/solmodel"
)

// PriceRange 表示价格范围
type PriceRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// PoolPriceRanges 池子的价格范围（需要包含 UpdatedAt 用于时间窗口检查）
type PoolPriceRanges struct {
	PoolState string                 `json:"pool_state"`
	Ranges    map[string]*PriceRange `json:"ranges"` // key: "24H", "7D", "30D"
	UpdatedAt time.Time              `json:"updated_at"`
}

// PriceRangeCacheReader 价格范围缓存读取器（用于 market 服务）
type PriceRangeCacheReader struct {
	redis   *redis.Redis
	chainID int64
}

// NewPriceRangeCacheReader 创建价格范围缓存读取器
func NewPriceRangeCacheReader(redisClient *redis.Redis, chainID int64) *PriceRangeCacheReader {
	return &PriceRangeCacheReader{
		redis:   redisClient,
		chainID: chainID,
	}
}

// GetPriceRanges 从 Redis 获取池子的价格范围
// 在获取时检查并过滤过期数据，确保返回的数据在时间窗口内
func (r *PriceRangeCacheReader) GetPriceRanges(ctx context.Context, poolState string) (priceRange24HMin, priceRange24HMax, priceRange7DMin, priceRange7DMax, priceRange30DMin, priceRange30DMax float64) {
	if r.redis == nil {
		return 0, 0, 0, 0, 0, 0
	}

	key := r.getRedisKey(poolState)
	val, err := r.redis.Get(key)
	if err != nil {
		// Redis 中没有数据，返回 0
		return 0, 0, 0, 0, 0, 0
	}

	// 检查数据是否为空或无效
	if val == "" || len(val) == 0 {
		logx.Infof("PriceRangeCacheReader: empty data for key %s, deleting corrupted key", key)
		// 删除损坏的 key
		_, _ = r.redis.Del(key)
		return 0, 0, 0, 0, 0, 0
	}

	var ranges PoolPriceRanges
	if err := json.Unmarshal([]byte(val), &ranges); err != nil {
		logx.Errorf("PriceRangeCacheReader: failed to unmarshal data for key %s, data length: %d, error: %v. Deleting corrupted key.", key, len(val), err)
		// 删除损坏的数据，避免重复错误
		_, _ = r.redis.Del(key)
		return 0, 0, 0, 0, 0, 0
	}

	// 验证数据完整性
	if ranges.Ranges == nil {
		logx.Infof("PriceRangeCacheReader: invalid data structure for key %s (Ranges is nil), deleting corrupted key", key)
		_, _ = r.redis.Del(key)
		return 0, 0, 0, 0, 0, 0
	}

	// 检查并过滤过期数据，确保时间窗口准确
	now := time.Now()
	updatedAt := ranges.UpdatedAt

	// 只返回在时间窗口内的数据
	if r24H, ok := ranges.Ranges["24H"]; ok && r24H != nil {
		// 检查 24H 范围是否在时间窗口内
		if updatedAt.After(now.Add(-24 * time.Hour)) {
			priceRange24HMin = r24H.Min
			priceRange24HMax = r24H.Max
		}
	}
	if r7D, ok := ranges.Ranges["7D"]; ok && r7D != nil {
		// 检查 7D 范围是否在时间窗口内
		if updatedAt.After(now.Add(-7 * 24 * time.Hour)) {
			priceRange7DMin = r7D.Min
			priceRange7DMax = r7D.Max
		}
	}
	if r30D, ok := ranges.Ranges["30D"]; ok && r30D != nil {
		// 检查 30D 范围是否在时间窗口内
		if updatedAt.After(now.Add(-30 * 24 * time.Hour)) {
			priceRange30DMin = r30D.Min
			priceRange30DMax = r30D.Max
		}
	}

	return
}

// getRedisKey 获取 Redis key
func (r *PriceRangeCacheReader) getRedisKey(poolState string) string {
	return fmt.Sprintf("clmm_price_range:%d:%s", r.chainID, poolState)
}

type ServiceContext struct {
	Config               config.Config
	DB                   *gorm.DB
	RDS                  *redis.Redis
	SolCli               *ag_rpc.Client
	SolTokenAccountModel solmodel.SolTokenAccountModel
	ClmmPositionModel    solmodel.ClmmPositionModel
	PriceRangeCache      *PriceRangeCacheReader
}

func NewServiceContext(c config.Config) *ServiceContext {
	masterDsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		c.Mysql.Master.Username, c.Mysql.Master.Password, c.Mysql.Master.Path, c.Mysql.Master.Port, c.Mysql.Master.Dbname)
	db, err := gorm.Open(mysql.Open(masterDsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		panic(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}

	sqlDB.SetMaxOpenConns(c.Mysql.Master.MaxOpenConns)
	sqlDB.SetMaxIdleConns(c.Mysql.Master.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err = sqlDB.Ping(); err != nil {
		panic(err)
	}

	rds := redis.MustNewRedis(redis.RedisConf{
		Host:        c.Redis.Host,
		Type:        c.Redis.Type,
		Pass:        c.Redis.Pass,
		Tls:         c.Redis.Tls,
		PingTimeout: c.Redis.PingTimeout,
	})
	if !rds.Ping() {
		panic("rds ping err")
	}

	var solCli *ag_rpc.Client
	if c.Sol.Enable && len(c.Sol.NodeUrl) > 0 {
		solCli = ag_rpc.New(c.Sol.NodeUrl[0])
	}

	return &ServiceContext{
		DB:                   db,
		Config:               c,
		RDS:                  rds,
		SolCli:               solCli,
		SolTokenAccountModel: solmodel.NewSolTokenAccountModel(db),
		ClmmPositionModel:    solmodel.NewClmmPositionModel(db),
		PriceRangeCache:      NewPriceRangeCacheReader(rds, constants.SolChainIdInt),
	}
}
