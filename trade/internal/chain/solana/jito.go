package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpc"
	"richcode.cc/dex/pkg/backoff"
)

var (
	TipAddress = aSDK.MustPublicKeyFromBase58("96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5")
	//jito addr 96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5
	//bloxroute addr HWEoBxYs7ssKuudEjzjmpfJVX7Dvi7wescFsVx2L5yoY
)

const (
	bundleEndpoint    = "https://bundles.jito.wtf/v1/bundles"
	jitoTxEndpoint    = "https://mainnet.block-engine.jito.wtf/api/v1/transactions"
	JitoRateLimitCode = "32097"
)

type BundleRequest struct {
	Transactions [][]byte `json:"transactions"` // Serialized transactions
	UUID         string   `json:"uuid"`         // Jito UUID for authentication
}

type JitoTipFloor struct {
	Time                        time.Time `json:"time"`
	LandedTips25ThPercentile    float64   `json:"landed_tips_25th_percentile"`
	LandedTips50ThPercentile    float64   `json:"landed_tips_50th_percentile"`
	LandedTips75ThPercentile    float64   `json:"landed_tips_75th_percentile"`
	LandedTips95ThPercentile    float64   `json:"landed_tips_95th_percentile"`
	LandedTips99ThPercentile    float64   `json:"landed_tips_99th_percentile"`
	EmaLandedTips50ThPercentile float64   `json:"ema_landed_tips_50th_percentile"`
}

func (tm *TxManager) queryJitoRpc(ctx context.Context) (*JitoTipFloor, error) {
	resp, err := httpc.Do(ctx, http.MethodGet, "https://bundles.jito.wtf/api/v1/bundles/tip_floor", nil)
	if err != nil {
		return nil, err
	}
	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var jitoTipFloor []JitoTipFloor
	err = json.Unmarshal(res, &jitoTipFloor)
	if nil != err {
		return nil, err
	}
	if len(jitoTipFloor) == 0 {
		return nil, nil
	}
	return &jitoTipFloor[0], nil
}

// curl https://bundles.jito.wtf/api/v1/bundles/tip_floor
func (tm *TxManager) ListJitoFloorFee() float64 {
	tm.RWLock.RLock()
	defer tm.RWLock.RUnlock()
	return tm.jitoTipFloor.LandedTips50ThPercentile
}

func (tm *TxManager) CheckJitoFloorFee() {
	tm.updateJitoFloorFee()

	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-ticker.C:
			tm.updateJitoFloorFee()
		}
	}
}

func (tm *TxManager) updateJitoFloorFee() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := tm.queryJitoRpc(ctx)
	if err != nil {
		logx.Error(err)
		return
	}
	tm.RWLock.Lock()
	tm.jitoTipFloor = res
	tm.RWLock.Unlock()
}

func (tm *TxManager) SendViaJito(ctx context.Context, tx *aSDK.Transaction) (string, error) {
	// jito是不会校验的 所以需要 SimulateTransaction
	err := tm.simulate(ctx, tx)
	if err != nil {
		return "", err
	}

	sig, err := tm.JitoClient.SendTransaction(ctx, tx)
	if nil != err {
		return "", err
	}

	return sig.String(), nil
}

func (tm *TxManager) SendViaJitoRetry(ctx context.Context, tx *aSDK.Transaction) (string, error) {
	// jito是不会校验的 所以需要 SimulateTransaction
	var err error
	err = tm.simulate(ctx, tx)
	if err != nil {
		return "", err
	}
	txhash := ""
	for i := 0; i < 5; i++ {
		var sig aSDK.Signature
		sig, err = tm.JitoClient.SendTransaction(ctx, tx)
		if err != nil {
			if !strings.Contains(err.Error(), JitoRateLimitCode) {
				logc.Error(ctx, err)
				return "", err
			}
			logc.Info(ctx, err)
			time.Sleep(backoff.DefaultExpoential.Backoff(i))
			continue
		}
		txhash = sig.String()
		break
	}

	return txhash, err
}

func (tm *TxManager) SendViaJitoBundles(ctx context.Context, tx *aSDK.Transaction) (string, error) {
	txSerialized, err := tx.MarshalBinary()
	if err != nil {
		return "", err
	}

	bundle := BundleRequest{
		Transactions: [][]byte{txSerialized},
		UUID:         tm.JitoUUID,
	}

	// Submit the bundle
	response, err := submitBundle(ctx, bundleEndpoint, bundle)
	if err != nil {

		return "", err
	}

	return response, nil
}

// Helper function to submit a bundle
func submitBundle(ctx context.Context, endpoint string, bundle BundleRequest) (string, error) {
	jsonPayload, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("failed to serialize bundle request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	response, _ := json.MarshalIndent(result, "", "  ")
	return string(response), nil
}
