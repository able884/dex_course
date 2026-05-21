package block

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"

	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/sol"
	"richcode.cc/dex/pkg/types"
)

// TokenMetadata 保存 Token 相关的元数据
type TokenMetadata struct {
	Address  string
	Decimals uint8
	Slot     int64
}

type tokenMetadataCachePayload struct {
	Symbol          string  `json:"symbol,omitempty"`
	Name            string  `json:"name,omitempty"`
	Program         string  `json:"program,omitempty"`
	TwitterUsername string  `json:"twitter,omitempty"`
	Website         string  `json:"website,omitempty"`
	Telegram        string  `json:"telegram,omitempty"`
	Icon            string  `json:"icon,omitempty"`
	Description     string  `json:"description,omitempty"`
	TotalSupply     float64 `json:"totalSupply,omitempty"`
	LastSuccessTs   int64   `json:"lastSuccessTs,omitempty"`
	LastFailureTs   int64   `json:"lastFailureTs,omitempty"`
}

// SaveToken 根据成交信息保存 Token 和 BaseToken 的元数据
func (s *BlockService) SaveToken(ctx context.Context, trade *types.TradeWithPair) (tokenDB *solmodel.Token, err error) {
	// 保存主 Token
	token, err := s.saveTokenByAddress(ctx, &TokenMetadata{
		Address:  trade.PairInfo.TokenAddr,
		Decimals: trade.PairInfo.TokenDecimal,
		Slot:     trade.Slot,
	})
	if err != nil {
		s.Errorf("SaveToken: Failed to save token %s: %v", trade.PairInfo.TokenAddr, err)
		return nil, err
	}

	// 保存 BaseToken
	_, err = s.saveTokenByAddress(ctx, &TokenMetadata{
		Address:  trade.PairInfo.BaseTokenAddr,
		Decimals: trade.PairInfo.BaseTokenDecimal,
		Slot:     trade.Slot,
	})
	if err != nil {
		s.Errorf("SaveToken: Failed to save base token %s: %v", trade.PairInfo.BaseTokenAddr, err)
		// BaseToken 保存失败不影响主 Token 的返回
	}

	return token, nil
}

// saveTokenByAddress 保存指定地址的 Token 元数据
func (s *BlockService) saveTokenByAddress(ctx context.Context, metadata *TokenMetadata) (tokenDB *solmodel.Token, err error) {
	if metadata == nil || metadata.Address == "" {
		return nil, fmt.Errorf("metadata is nil or address is empty")
	}
	if s.sc == nil || s.sc.TokenModel == nil {
		return nil, fmt.Errorf("service context is nil")
	}
	if len(s.sc.Config.Sol.NodeUrl) == 0 {
		return nil, fmt.Errorf("solana configuration is missing or invalid")
	}

	tokenModel := s.sc.TokenModel
	chainId := SolChainIdInt

	tokenDB, err = tokenModel.FindOneByChainIdAddress(ctx, int64(chainId), metadata.Address)
	if err != nil && !errors.Is(err, solmodel.ErrNotFound) && !strings.Contains(err.Error(), "record not found") {
		return nil, fmt.Errorf("saveTokenByAddress: unexpected find error: %w", err)
	}

	cachedPayload, cacheErr := s.loadCachedTokenMetadata(ctx, metadata.Address)
	if cacheErr != nil {
		s.Infof("saveTokenByAddress: metadata cache unavailable for %s: %v", metadata.Address, cacheErr)
	}

	solClient := s.sc.GetSolClient()
	opts := &jsonrpc.RPCClientOpts{
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	rpcClient := jsonrpc.NewClientWithOpts(s.sc.Config.Sol.NodeUrl[0], opts)

	if err == nil && tokenDB != nil {
		changed := false
		if s.applyCachedMetadata(tokenDB, cachedPayload) {
			changed = true
		}
		if tokenDB.Slot == 0 {
			tokenDB.Slot = metadata.Slot
			changed = true
		}
		if tokenDB.TotalSupply == 0 {
			if cachedPayload != nil && cachedPayload.TotalSupply > 0 {
				tokenDB.TotalSupply = cachedPayload.TotalSupply
				changed = true
			} else if totalSupply, supplyErr := sol.GetTokenTotalSupply(solClient, s.ctx, tokenDB.Address); supplyErr == nil {
				tokenDB.TotalSupply = totalSupply.InexactFloat64()
				changed = true
			} else {
				s.Errorf("saveTokenByAddress:GetTokenTotalSupply update err:%v, address: %v", supplyErr, tokenDB.Address)
				s.cacheTokenMetadataFailure(ctx, tokenDB.Address, tokenDB)
			}
		}
		if len(tokenDB.Program) == 0 {
			if program, progErr := sol.GetTokenProgram(solClient, s.ctx, tokenDB.Address); progErr == nil {
				switch program {
				case common.TokenProgramID:
					tokenDB.Program = common.TokenProgramID.String()
					changed = true
				case common.Token2022ProgramID:
					tokenDB.Program = common.Token2022ProgramID.String()
					changed = true
				}
			}
		}
		if len(tokenDB.Symbol) == 0 || len(tokenDB.Name) == 0 {
			switch tokenDB.Program {
			case common.TokenProgramID.String():
				if tokenInfo, infoErr := sol.GetTokenInfo(solClient, s.ctx, tokenDB.Address); infoErr == nil && tokenInfo != nil {
					tokenDB.Symbol = tokenInfo.Data.Symbol
					tokenDB.Name = tokenInfo.Data.Name
					tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
					tokenDB.Website = tokenInfo.Uri.Website
					tokenDB.Telegram = tokenInfo.Uri.Telegram
					tokenDB.Icon = tokenInfo.Uri.Image
					tokenDB.Description = tokenInfo.Uri.Description
					if len(tokenInfo.Uri.Symbol) > 0 {
						tokenDB.Symbol = tokenInfo.Uri.Symbol
					}
					if len(tokenInfo.Uri.Name) > 0 {
						tokenDB.Name = tokenInfo.Uri.Name
					}
					changed = true
					s.cacheTokenMetadataSuccess(ctx, tokenDB.Address, tokenDB)
				} else {
					s.Errorf("saveTokenByAddress:GetTokenInfo update err: %v, address: %v", infoErr, tokenDB.Address)
					s.cacheTokenMetadataFailure(ctx, tokenDB.Address, tokenDB)
				}
			case common.Token2022ProgramID.String():
				_, tokenInfo, infoErr := sol.GetToken2022Info(ag_rpc.NewWithCustomRPCClient(rpcClient), s.ctx, solana.MustPublicKeyFromBase58(tokenDB.Address))
				if infoErr == nil && tokenInfo != nil {
					tokenDB.Symbol = tokenInfo.Data.Symbol
					tokenDB.Name = tokenInfo.Data.Name
					tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
					tokenDB.Website = tokenInfo.Uri.Website
					tokenDB.Telegram = tokenInfo.Uri.Telegram
					tokenDB.Icon = tokenInfo.Uri.Image
					tokenDB.Description = tokenInfo.Uri.Description
					if len(tokenInfo.Uri.Name) > 0 {
						tokenDB.Name = tokenInfo.Uri.Name
					}
					if len(tokenInfo.Uri.Symbol) > 0 {
						tokenDB.Symbol = tokenInfo.Uri.Symbol
					}
					changed = true
					s.cacheTokenMetadataSuccess(ctx, tokenDB.Address, tokenDB)
				} else {
					s.Errorf("saveTokenByAddress:GetToken2022Info err: %v, token address: %v", infoErr, tokenDB.Address)
					s.cacheTokenMetadataFailure(ctx, tokenDB.Address, tokenDB)
				}
			}
		}
		if changed {
			if updateErr := tokenModel.Update(s.ctx, tokenDB); updateErr != nil {
				return nil, fmt.Errorf("saveTokenByAddress: update token err: %w", updateErr)
			}
			s.cacheTokenMetadataSuccess(ctx, tokenDB.Address, tokenDB)
		}
		return tokenDB, nil
	}

	if errors.Is(err, solmodel.ErrNotFound) || (err != nil && strings.Contains(err.Error(), "record not found")) {
		tokenDB = &solmodel.Token{
			ChainId:  int64(chainId),
			Address:  metadata.Address,
			Decimals: int64(metadata.Decimals),
			Slot:     metadata.Slot,
		}
		s.applyCachedMetadata(tokenDB, cachedPayload)

		if tokenDB.TotalSupply == 0 {
			if cachedPayload != nil && cachedPayload.TotalSupply > 0 {
				tokenDB.TotalSupply = cachedPayload.TotalSupply
			} else if totalSupply, supplyErr := sol.GetTokenTotalSupply(solClient, s.ctx, tokenDB.Address); supplyErr == nil {
				tokenDB.TotalSupply = totalSupply.InexactFloat64()
			} else {
				s.Errorf("saveTokenByAddress:GetTokenTotalSupply insert err:%v, address: %v", supplyErr, tokenDB.Address)
				s.cacheTokenMetadataFailure(ctx, tokenDB.Address, tokenDB)
			}
		}

		program, _ := sol.GetTokenProgram(solClient, s.ctx, tokenDB.Address)
		switch program {
		case common.Token2022ProgramID:
			tokenDB.Program = common.Token2022ProgramID.String()
			_, tokenInfo, infoErr := sol.GetToken2022Info(ag_rpc.NewWithCustomRPCClient(rpcClient), s.ctx, solana.MustPublicKeyFromBase58(tokenDB.Address))
			if infoErr == nil && tokenInfo != nil {
				tokenDB.Symbol = tokenInfo.Data.Symbol
				tokenDB.Name = tokenInfo.Data.Name
				tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
				tokenDB.Website = tokenInfo.Uri.Website
				tokenDB.Telegram = tokenInfo.Uri.Telegram
				tokenDB.Icon = tokenInfo.Uri.Image
				tokenDB.Description = tokenInfo.Uri.Description
				if len(tokenInfo.Uri.Name) > 0 {
					tokenDB.Name = tokenInfo.Uri.Name
				}
				if len(tokenInfo.Uri.Symbol) > 0 {
					tokenDB.Symbol = tokenInfo.Uri.Symbol
				}
				s.cacheTokenMetadataSuccess(ctx, tokenDB.Address, tokenDB)
			} else {
				s.Errorf("saveTokenByAddress:GetToken2022Info err: %v, token address: %v", infoErr, tokenDB.Address)
				s.cacheTokenMetadataFailure(ctx, tokenDB.Address, tokenDB)
			}
		default:
			tokenDB.Program = common.TokenProgramID.String()
			tokenInfo, infoErr := sol.GetTokenInfo(solClient, s.ctx, tokenDB.Address)
			if infoErr == nil && tokenInfo != nil {
				tokenDB.Symbol = tokenInfo.Data.Symbol
				tokenDB.Name = tokenInfo.Data.Name
				tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
				tokenDB.Website = tokenInfo.Uri.Website
				tokenDB.Telegram = tokenInfo.Uri.Telegram
				tokenDB.Icon = tokenInfo.Uri.Image
				tokenDB.Description = tokenInfo.Uri.Description
				if len(tokenInfo.Uri.Symbol) > 0 {
					tokenDB.Symbol = tokenInfo.Uri.Symbol
				}
				if len(tokenInfo.Uri.Name) > 0 {
					tokenDB.Name = tokenInfo.Uri.Name
				}
				s.cacheTokenMetadataSuccess(ctx, tokenDB.Address, tokenDB)
			} else {
				s.Errorf("saveTokenByAddress:GetTokenInfo err: %v, address: %v", infoErr, tokenDB.Address)
				s.cacheTokenMetadataFailure(ctx, tokenDB.Address, tokenDB)
			}
		}

		if insertErr := tokenModel.Insert(ctx, tokenDB); insertErr != nil {
			if strings.Contains(insertErr.Error(), "Duplicate entry") {
				tokenDB, err = tokenModel.FindOneByChainIdAddress(ctx, int64(chainId), metadata.Address)
				if err != nil {
					return nil, err
				}
				return tokenDB, nil
			}
			return nil, insertErr
		}
		return tokenDB, nil
	}

	return nil, fmt.Errorf("saveTokenByAddress: unexpected error from FindOneByChainIdAddress: %w", err)
}

func (s *BlockService) applyCachedMetadata(token *solmodel.Token, payload *tokenMetadataCachePayload) bool {
	if token == nil || payload == nil {
		return false
	}
	changed := false
	if token.Symbol == "" && payload.Symbol != "" {
		token.Symbol = payload.Symbol
		changed = true
	}
	if token.Name == "" && payload.Name != "" {
		token.Name = payload.Name
		changed = true
	}
	if token.Program == "" && payload.Program != "" {
		token.Program = payload.Program
		changed = true
	}
	if token.TwitterUsername == "" && payload.TwitterUsername != "" {
		token.TwitterUsername = payload.TwitterUsername
		changed = true
	}
	if token.Website == "" && payload.Website != "" {
		token.Website = payload.Website
		changed = true
	}
	if token.Telegram == "" && payload.Telegram != "" {
		token.Telegram = payload.Telegram
		changed = true
	}
	if token.Icon == "" && payload.Icon != "" {
		token.Icon = payload.Icon
		changed = true
	}
	if token.Description == "" && payload.Description != "" {
		token.Description = payload.Description
		changed = true
	}
	if token.TotalSupply == 0 && payload.TotalSupply > 0 {
		token.TotalSupply = payload.TotalSupply
		changed = true
	}
	return changed
}

func (s *BlockService) loadCachedTokenMetadata(ctx context.Context, address string) (*tokenMetadataCachePayload, error) {
	if s.metadataCache == nil {
		return nil, nil
	}
	key := TokenMetadataCacheKey(SolChainIdInt, address)
	if res, ok := s.metadataCacheGet(key); ok {
		return res.payload, res.err
	}
	raw, err := s.metadataCache.Get(ctx, key)
	if err != nil || raw == "" {
		s.metadataCacheSetResult(key, &metadataCacheResult{payload: nil, err: err})
		return nil, err
	}
	var payload tokenMetadataCachePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		s.metadataCacheSetResult(key, &metadataCacheResult{payload: nil, err: err})
		return nil, err
	}
	var cooldownErr error
	if payload.LastFailureTs > 0 {
		if time.Since(time.Unix(payload.LastFailureTs, 0)) < s.metadataFailureCooldown {
			cooldownErr = fmt.Errorf("metadata fetch suppressed due to cooldown")
		}
	}
	s.metadataCacheSetResult(key, &metadataCacheResult{payload: &payload, err: cooldownErr})
	return &payload, cooldownErr
}

func (s *BlockService) cacheTokenMetadataSuccess(ctx context.Context, address string, token *solmodel.Token) {
	if s.metadataCache == nil || token == nil {
		return
	}
	payload := payloadFromToken(token)
	if payload == nil {
		return
	}
	payload.LastSuccessTs = time.Now().Unix()
	payload.LastFailureTs = 0
	s.writeMetadataCache(ctx, address, payload)
}

func (s *BlockService) cacheTokenMetadataFailure(ctx context.Context, address string, token *solmodel.Token) {
	if s.metadataCache == nil {
		return
	}
	payload := payloadFromToken(token)
	if payload == nil {
		payload = &tokenMetadataCachePayload{}
	}
	payload.LastFailureTs = time.Now().Unix()
	s.writeMetadataCache(ctx, address, payload)
}

func (s *BlockService) writeMetadataCache(ctx context.Context, address string, payload *tokenMetadataCachePayload) {
	if s.metadataCache == nil || payload == nil {
		return
	}
	key := TokenMetadataCacheKey(SolChainIdInt, address)
	body, err := json.Marshal(payload)
	if err != nil {
		s.Errorf("writeMetadataCache: marshal err %v", err)
		return
	}
	ttl := s.metadataTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if err := s.metadataCache.Set(ctx, key, string(body), ttl); err != nil {
		s.Errorf("writeMetadataCache: set err %v", err)
		return
	}
	s.metadataCacheSetResult(key, &metadataCacheResult{payload: payload})
}

func payloadFromToken(token *solmodel.Token) *tokenMetadataCachePayload {
	if token == nil {
		return nil
	}
	return &tokenMetadataCachePayload{
		Symbol:          token.Symbol,
		Name:            token.Name,
		Program:         token.Program,
		TwitterUsername: token.TwitterUsername,
		Website:         token.Website,
		Telegram:        token.Telegram,
		Icon:            token.Icon,
		Description:     token.Description,
		TotalSupply:     token.TotalSupply,
	}
}
