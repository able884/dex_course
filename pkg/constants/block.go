package constants

const (
	BlockPending   = 0 // 待处理
	BlockProcessed = 1 // 已处理
	BlockFailed    = 2 // 处理失败
	// BlockSkipped GetBlock err:{"code":-32007,"message":"Slot 311350484 was skipped, or missing due to ledger jump to recent snapshot","data":null}
	BlockSkipped = 3 // 跳过
)

// RetryBackoffSeconds 重试退避时间（秒）：5s → 30s → 120s → 300s
var RetryBackoffSeconds = []int{5, 30, 120, 300}

// MaxRetryCount 最大重试次数
const MaxRetryCount = 3
