package rediskeys

import (
	"fmt"
	"strings"
	"time"
)

const (
	pumpPairCreatedKeyFmt = "pump:create:pair:%d:%s"
)

// PumpPairCreatedTTL defines how long a pair creation marker stays valid.
const PumpPairCreatedTTL = 10 * time.Minute

// PumpPairCreatedKey formats the redis key used to mark a pump pair creation in progress.
func PumpPairCreatedKey(chainId int64, pairAddr string) string {
	return fmt.Sprintf(pumpPairCreatedKeyFmt, chainId, strings.ToLower(pairAddr))
}
