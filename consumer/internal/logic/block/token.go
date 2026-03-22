package block

import (
	"context"
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

// SaveToken 根据成交信息补全或落库 Token 元数据，包含总量、符号、社交信息等。
func (s *BlockService) SaveToken(ctx context.Context, trade *types.TradeWithPair) (tokenDB *solmodel.Token, err error) {
	s.Infof("SaveToken: Starting with trade.PairInfo.TokenAddr: %v", trade.PairInfo.TokenAddr)

	// Check if trade is nil or if PairInfo has empty TokenAddr
	if trade == nil || trade.PairInfo.TokenAddr == "" {
		s.Errorf("SaveToken: trade is nil or PairInfo.TokenAddr is empty")
		return nil, fmt.Errorf("trade is nil or PairInfo.TokenAddr is empty")
	}

	// Check if service context and required models are avail	able
	if s.sc == nil {
		s.Errorf("SaveToken: service context is nil")
		return nil, fmt.Errorf("service context is nil")
	}

	if s.sc.TokenModel == nil {
		s.Errorf("SaveToken: TokenModel is nil")
		return nil, fmt.Errorf("TokenModel is nil")
	}

	if len(s.sc.Config.Sol.NodeUrl) == 0 {
		s.Errorf("SaveToken: Solana configuration is missing or invalid")
		return nil, fmt.Errorf("Solana configuration is missing or invalid")
	}

	// Check if context is available
	if s.ctx == nil {
		s.Errorf("SaveToken: service context is nil")
		return nil, fmt.Errorf("service context is nil")
	}

	s.Infof("SaveToken: All checks passed, proceeding with database query")

	tokenModel := s.sc.TokenModel
	chainId := SolChainIdInt
	s.Infof("SaveToken: Calling FindOneByChainIdAddress with chainId: %v, tokenAddr: %v", chainId, trade.PairInfo.TokenAddr)

	tokenDB, err = tokenModel.FindOneByChainIdAddress(ctx, int64(chainId), trade.PairInfo.TokenAddr)

	s.Infof("SaveToken: FindOneByChainIdAddress result - err: %v, tokenDB: %v", err, tokenDB != nil)

	if err != nil {
		s.Infof("SaveToken: Error details - %T: %v", err, err)
		// Check for record not found error - handle both specific error type and string matching
		if errors.Is(err, solmodel.ErrNotFound) || strings.Contains(err.Error(), "record not found") {
			s.Infof("SaveToken: Token not found, will create new token")
		} else {
			s.Errorf("SaveToken: Unexpected error from FindOneByChainIdAddress: %v", err)
		}
	}

	solClient := s.sc.GetSolClient()

	opts := &jsonrpc.RPCClientOpts{
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	rpcClient := jsonrpc.NewClientWithOpts(s.sc.Config.Sol.NodeUrl[0], opts)

	if err == nil && tokenDB != nil {
		// 分支一：数据库中已有记录，按需补全缺失的元数据
		s.Infof("SaveToken: Token found in database, updating existing token: %v", tokenDB.Address)

		change := false

		if tokenDB.Slot == 0 {
			tokenDB.Slot = trade.Slot
			change = true
			s.Infof("SaveToken: Updated slot to: %v", trade.Slot)
		}

		if tokenDB.TotalSupply == 0 {
			// 仅在总量缺失时请求链上，减少 RPC 次数
			totalSupply, err := sol.GetTokenTotalSupply(solClient, s.ctx, tokenDB.Address)
			s.Infof("SaveToken:GetTokenMeta update token totalSupply: token addr: %v, totalSupply: %v", tokenDB.Address, totalSupply)
			if err == nil {
				tokenDB.TotalSupply = totalSupply.InexactFloat64()
				change = true
			} else {
				s.Errorf("SaveToken:GetTokenTotalSupply update err:%v, address: %v", err, tokenDB.Address)
			}
		}
		if len(tokenDB.Program) == 0 {
			// 优先探测 TokenProgram，用于后续选择不同的元数据解析方案
			program, _ := sol.GetTokenProgram(solClient, s.ctx, tokenDB.Address)
			switch program {
			case common.TokenProgramID:
				tokenDB.Program = common.TokenProgramID.String()
				change = true
			case common.Token2022ProgramID:
				tokenDB.Program = common.Token2022ProgramID.String()
				change = true
			default:

			}
		}
		// todo: support token 2022 https://solscan.io/token/7atgF8KQo4wJrD5ATGX7t1V2zVvykPJbFfNeVf1icFv1#metadata
		if len(tokenDB.Symbol) == 0 || len(tokenDB.Name) == 0 {
			// 兜底场景：从链上元数据补齐符号、名称及社交链接
			switch tokenDB.Program {
			case common.TokenProgramID.String():
				tokenInfo, err := sol.GetTokenInfo(solClient, s.ctx, tokenDB.Address)
				if err != nil {
					s.Errorf("SaveToken:GetTokenInfo update err: %v, address: %v", err, tokenDB.Address)
				}
				if tokenInfo != nil {
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

					change = true
					s.Infof("update parse token address: %v,result: %v", tokenDB.Address, tokenDB)
				}
			case common.Token2022ProgramID.String():

				_, tokenInfo, err := sol.GetToken2022Info(ag_rpc.NewWithCustomRPCClient(rpcClient), s.ctx, solana.MustPublicKeyFromBase58(tokenDB.Address))
				if err != nil {
					s.Errorf("SaveToken:GetToken2022Info err: %v, token address: %v", err, tokenDB.Address)
				}

				if tokenInfo != nil {

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

					change = true

					s.Infof("update parse token2022 address: %v,result: %v", tokenDB.Address, tokenDB)
				}

			default:
			}

		}

		if change {
			s.Infof("SaveToken: Updating existing token in database")
			_ = tokenModel.Update(s.ctx, tokenDB)
		}

		//更新成功
		s.Infof("SaveToken: Successfully updated existing token: %v", tokenDB.Address)
		return tokenDB, nil
	}

	if errors.Is(err, solmodel.ErrNotFound) || (err != nil && strings.Contains(err.Error(), "record not found")) {
		// 分支二：首次出现的 Token，构造基础信息后再补全链上元数据
		s.Infof("SaveToken: Creating new token for address: %v", trade.PairInfo.TokenAddr)

		tokenDB = &solmodel.Token{
			ChainId:  int64(chainId),
			Address:  trade.PairInfo.TokenAddr,
			Decimals: int64(trade.PairInfo.TokenDecimal),
			Slot:     trade.Slot,
		}

		s.Infof("SaveToken: Created token struct with ChainId: %v, Address: %v, Decimals: %v, Slot: %v",
			tokenDB.ChainId, tokenDB.Address, tokenDB.Decimals, tokenDB.Slot)

		totalSupply, err := sol.GetTokenTotalSupply(solClient, s.ctx, tokenDB.Address)
		if err == nil {
			tokenDB.TotalSupply = totalSupply.InexactFloat64()
		} else {
			s.Errorf("SaveToken:GetTokenTotalSupply insert err:%v, address: %v", err, tokenDB.Address)
		}

		program, _ := sol.GetTokenProgram(solClient, s.ctx, tokenDB.Address)
		switch program {
		case common.Token2022ProgramID:
			tokenDB.Program = common.Token2022ProgramID.String()

			_, tokenInfo, err := sol.GetToken2022Info(ag_rpc.NewWithCustomRPCClient(rpcClient), s.ctx, solana.MustPublicKeyFromBase58(tokenDB.Address))
			if err != nil {
				s.Errorf("SaveToken:GetToken2022Info err: %v, token address: %v", err, tokenDB.Address)
			}

			if tokenInfo != nil {

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
			}
			s.Infof("insert parse token2022 address: %v,result: %v", tokenDB.Address, tokenDB)
		default:
			tokenDB.Program = common.TokenProgramID.String()

			// todo: error 	SaveToken:GetTokenInfo nil,insert , err: GetTokenInfo:GetAccountInfo token data is nil,
			tokenInfo, err := sol.GetTokenInfo(solClient, s.ctx, tokenDB.Address)
			if err != nil {
				s.Errorf("SaveToken:GetTokenInfo err: %v, address: %v", err, tokenDB.Address)
			}
			if tokenInfo != nil {
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
				// tokenDB.SetSolTokenDefaultCa()
				// tokenDB.IsCanAddToken = int64(tokenInfo.IsCanAddToken)
			}
			s.Infof("insert parse token address: %v,result: %v", tokenDB.Address, tokenDB)
		}

		err = tokenModel.Insert(ctx, tokenDB)
		if err != nil {
			s.Errorf("SaveToken: Insert failed with error: %v", err)
			if strings.Contains(err.Error(), "Duplicate entry") {
				s.Infof("SaveToken: Duplicate entry detected, fetching existing token")
				// db already exists
				tokenDB, err = tokenModel.FindOneByChainIdAddress(ctx, int64(chainId), trade.PairInfo.TokenAddr)
				if err != nil {
					s.Errorf("SaveToken: Failed to fetch existing token after duplicate: %v", err)
					return nil, err
				}
				s.Infof("SaveToken: Successfully fetched existing token after duplicate")
				return tokenDB, nil
			}
			s.Errorf("SaveToken: Insert failed with non-duplicate error: %v", err)
			return nil, err
		}

		s.Infof("SaveToken: Successfully inserted new token: %v", tokenDB.Address)
		return tokenDB, nil
	}

	s.Errorf("SaveToken: Unexpected error case - err: %v, err type: %T", err, err)
	return nil, fmt.Errorf("SaveToken: unexpected error from FindOneByChainIdAddress: %w", err)
}
