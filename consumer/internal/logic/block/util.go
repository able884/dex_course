package block

import (
	"fmt"
	"math"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/config"
)

// RemoveMinAndMaxAndCalculateAverage 移除最小值和最大值后计算平均值
func RemoveMinAndMaxAndCalculateAverage(nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	if len(nums) == 1 {
		return nums[0]
	}
	if len(nums) == 2 {
		return (nums[0] + nums[1]) / 2
	}

	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	minIndex, maxIndex := -1, -1

	for i, num := range nums {
		if num < minVal {
			minVal = num
			minIndex = i
		}
		if num > maxVal {
			maxVal = num
			maxIndex = i
		}
	}

	var filteredNums []float64
	for i, num := range nums {
		if i != minIndex && i != maxIndex {
			filteredNums = append(filteredNums, num)
		}
	}

	sum := 0.0
	for _, num := range filteredNums {
		sum += num
	}
	average := sum / float64(len(filteredNums))

	return average
}

// MetricName builds the fully-qualified metric name using the configured namespace.
func MetricName(component, metric string) string {
	namespace := config.Cfg.BlockPipeline.Metrics.Namespace
	if namespace == "" {
		namespace = "block_persistence"
	}
	return fmt.Sprintf("%s.%s.%s", namespace, component, metric)
}

// PersistenceLogFields produces a consistent set of log fields for structured logging.
func PersistenceLogFields(slot uint64, pairAddr, tokenAddr, batchID string, attempt int) []logx.LogField {
	fields := []logx.LogField{
		logx.Field(LogFieldSlot, slot),
	}
	if pairAddr != "" {
		fields = append(fields, logx.Field(LogFieldPair, pairAddr))
	}
	if tokenAddr != "" {
		fields = append(fields, logx.Field(LogFieldToken, tokenAddr))
	}
	if batchID != "" {
		fields = append(fields, logx.Field(LogFieldBatchID, batchID))
	}
	if attempt >= 0 {
		fields = append(fields, logx.Field(LogFieldAttempt, attempt))
	}
	return fields
}

// TokenMetadataCacheKey returns the redis key for a token metadata cache entry.
func TokenMetadataCacheKey(chainID int64, mint string) string {
	return fmt.Sprintf("%s:%d:%s", RedisKeyTokenMetadataPrefix, chainID, strings.ToLower(mint))
}

// TokenAccountCacheKey builds the redis key used for token account snapshots.
func TokenAccountCacheKey(owner, tokenAccount string) string {
	return fmt.Sprintf("%s:%s:%s", RedisKeyTokenAccountPrefix, strings.ToLower(owner), strings.ToLower(tokenAccount))
}
