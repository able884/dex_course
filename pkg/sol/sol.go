package sol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/metaplex/token_metadata"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"

	"richcode.cc/dex/pkg/sol/token2022"
	"richcode.cc/dex/pkg/transfer"
)

type TokenUriData struct {
	Twitter     string     `json:"twitter"`
	Website     string     `json:"website"`
	Telegram    string     `json:"telegram"`
	Name        string     `json:"name"`
	Image       string     `json:"image"`
	Symbol      string     `json:"symbol"`
	Description string     `json:"description"`
	Extensions  Extensions `json:"extensions"`
}

type Extensions struct {
	Website  string `json:"website"`
	Twitter  string `json:"twitter"`
	Telegram string `json:"telegram"`
}

type TokenInfo struct {
	token.MintAccount
	token_metadata.Data
	MetaData      token_metadata.Metadata
	Uri           TokenUriData
	TotalSupply   decimal.Decimal
	IsCanAddToken uint8
	IsDropFreeze  uint8
	HoldersCount  int64
}

func GetTokenMintInfo(c *client.Client, ctx context.Context, address string) (tokenInfo *TokenInfo, err error) {
	resp, err := c.GetAccountInfoWithConfig(ctx, address, client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		err = fmt.Errorf("GetTokenMintInfo token err:%v, token address: %v", err, address)
		return
	}

	if len(resp.Data) == 0 {
		err = fmt.Errorf("GetTokenMintInfo:GetAccountInfoWithConfig token data is nil, err:%v, token address: %#v", err, address)
		return nil, err
	}

	mintAccount, err := token.MintAccountFromData(resp.Data[:82])
	if err != nil {
		err = fmt.Errorf("GetTokenMintInfo:MintAccountFromData err:%v, token address: %v", err, address)
		return
	}

	tokenInfo = &TokenInfo{
		MintAccount: mintAccount,
	}
	return
}

func GetTokenTotalSupply(c *client.Client, ctx context.Context, address string) (decimal.Decimal, error) {
	supplyModel, err := c.GetTokenSupplyWithConfig(ctx, address, client.GetTokenSupplyConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		err = fmt.Errorf("GetTokenTotalSupply token err:%v,token address: %v", err, address)
		return decimal.Zero, err
	}
	totalSupply := decimal.NewFromInt(int64(supplyModel.Amount)).Div(decimal.New(1, int32(supplyModel.Decimals)))
	return totalSupply, nil
}

func GetTokenProgram(c *client.Client, ctx context.Context, address string) (program common.PublicKey, err error) {
	resp, err := c.GetAccountInfoWithConfig(ctx, address, client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		err = fmt.Errorf("GetTokenMintInfo token err:%v, token address: %v", err, address)
		return
	}

	switch resp.Owner {
	case common.Token2022ProgramID:
		return common.Token2022ProgramID, nil
	case common.TokenProgramID:
		return common.TokenProgramID, nil
	}
	return common.SystemProgramID, errors.New("not support")
}

func GetTokenInfo(c *client.Client, ctx context.Context, address string) (tokenInfo *TokenInfo, err error) {
	resp, err := c.GetAccountInfoWithConfig(ctx, address, client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		err = fmt.Errorf("GetAccountInfoWithConfig token err:%v, token address: %v", err, address)
		return
	}

	if len(resp.Data) == 0 {
		err = fmt.Errorf("GetTokenInfo:GetAccountInfoWithConfig token data is nil, err:%v, token address: %#v", err, address)
		return nil, err
	}

	mintAccount, err := token.MintAccountFromData(resp.Data[:82])
	if err != nil {
		err = fmt.Errorf("GetTokenInfo:MintAccountFromData err:%v, token address: %v", err, address)
		return
	}

	tokenInfo = &TokenInfo{
		MintAccount: mintAccount,
	}
	// 在 solana 上，元数据账户是由 Token Metadata Program 管理的，该程序的地址是固定的：
	// Token Metadata Program ID: metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s
	// 元数据账户的地址是由代币的 Mint 地址和一组固定的种子（seeds）通过程序派生地址 (Program Derived Address, PDA) 的方式计算得出的。
	// 	1.	固定种子
	// Token Metadata Program 使用以下种子来派生元数据账户地址：
	//	•	metadata（固定字符串，表示账户类型）
	//	•	Token Metadata Program ID
	//	•	Token 的 Mint 地址
	//	2.	PDA 计算
	// 使用 Solana 的 PDA 规则，结合上述种子，计算出元数据账户地址。
	meta, err := token_metadata.GetTokenMetaPubkey(common.PublicKeyFromString(address))
	if err != nil {
		err = fmt.Errorf("GetTokenMetaPubkey err:%w", err)
		return
	}
	metaAccount, err := c.GetAccountInfoWithConfig(ctx, meta.String(), client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		err = fmt.Errorf("GetTokenInfo:GetAccountInfoWithConfig meta err:%w", err)
		return
	}
	if len(metaAccount.Data) <= 0 {
		return
	}
	metaData, err := token_metadata.MetadataDeserialize(metaAccount.Data)
	if err != nil {
		err = fmt.Errorf("deserialize metaAccount data err:%w", err)
		return
	}
	tokenInfo.MetaData = metaData
	tokenInfo.Data = metaData.Data

	if len(metaData.Data.Uri) > 0 {
		publicGateway := "https://ipfs.io/ipfs/"
		if !isURLAccessible(metaData.Data.Uri) {
			metaData.Data.Uri = replaceWithPublicGateway(metaData.Data.Uri, publicGateway)
		}

		ctx, cancelFunc := context.WithTimeout(context.Background(), 3000*time.Millisecond)
		defer cancelFunc()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, metaData.Data.Uri, nil)
		if err != nil {
			err = fmt.Errorf("http.NewRequest err:%w", err)
			return tokenInfo, err
		}

		// 执行请求
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			// skip error
			return tokenInfo, nil
		}
		defer func() {
			_ = response.Body.Close()
		}()

		res, err := io.ReadAll(response.Body)

		if err != nil {
			// skip error
			return tokenInfo, nil
		}
		// 检查 Content-Type
		contentType := response.Header.Get("Content-Type")
		if strings.Contains(string(res), "Account has been disabled.") {
			return tokenInfo, nil
		}
		if strings.HasPrefix(contentType, "application/json") {
			tokenUriData, err := transfer.Byte2Struct[TokenUriData](res)
			if err != nil {
				return tokenInfo, nil
			}

			if len(tokenUriData.Website) == 0 {
				tokenUriData.Website = tokenUriData.Extensions.Website
			}
			if len(tokenUriData.Telegram) == 0 {
				tokenUriData.Telegram = tokenUriData.Extensions.Telegram
			}
			if len(tokenUriData.Twitter) == 0 {
				tokenUriData.Twitter = tokenUriData.Extensions.Twitter
			}

			tokenInfo.Uri = tokenUriData
		} else if strings.HasPrefix(contentType, "image/") {
			// maybe picture
			// https://solscan.io/token/2HPtzSqkivqk8P5ySqVxB17b93sXsJN4s77kJp4Eish9#metadata
			// if strings.Contains(err.Error(), "invalid character") {
			// 	tokenInfo.Uri.Image = metaData.Data.Uri
			// }
			// skip error
			tokenInfo.Uri.Image = metaData.Data.Uri
		} else {
			// default
			tokenUriData, err := transfer.Byte2Struct[TokenUriData](res)
			if err != nil {
				// err = fmt.Errorf("GetTokenInfo error: %v, url: %v, token address: %v", err, metaData.Data.Uri, address)
				return tokenInfo, nil
			}

			if len(tokenUriData.Website) == 0 {
				tokenUriData.Website = tokenUriData.Extensions.Website
			}
			if len(tokenUriData.Telegram) == 0 {
				tokenUriData.Telegram = tokenUriData.Extensions.Telegram
			}
			if len(tokenUriData.Twitter) == 0 {
				tokenUriData.Twitter = tokenUriData.Extensions.Twitter
			}

			tokenInfo.Uri = tokenUriData

		}

	}

	return
}

// 检查 URL 是否可访问
func isURLAccessible(url string) bool {
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// 替换为公共的 IPFS 网关
func replaceWithPublicGateway(ipfsURL string, publicGateway string) string {
	// 正则表达式来匹配 IPFS 网关，您可以根据需要进行扩展
	pattern := `^https?://[^/]+/ipfs/`

	re := regexp.MustCompile(pattern)
	if re.MatchString(ipfsURL) {
		// 替换为公共网关
		return re.ReplaceAllString(ipfsURL, publicGateway)
	}
	return ipfsURL // 如果没有匹配，返回原始 URL
}

func GetToken2022Info(c *ag_rpc.Client, ctx context.Context, address solana.PublicKey) (token2022Info *token2022.Info, tokenInfo *TokenInfo, err error) {
	resp, err := c.GetAccountInfoWithOpts(ctx, address, &ag_rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingJSONParsed,
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil || resp == nil {
		err = fmt.Errorf("GetToken2022Info:GetAccountInfoWithOpts token err:%v, token address: %v", err, address)
		return
	}

	if len(resp.Value.Data.GetRawJSON()) == 0 {
		err = fmt.Errorf("GetToken2022Info:GetAccountInfoWithOpts token data is nil, err:%v, token address: %#v", err, address)
		return nil, nil, err
	}

	mintResponse, err := transfer.Byte2Struct[*token2022.MintResponse](resp.Value.Data.GetRawJSON())
	if err != nil {
		err = fmt.Errorf("GetToken2022Info:Byte2Struct err:%v, token address: %v", err, address)
		return
	}

	tokenInfo = &TokenInfo{}

	for _, extension := range mintResponse.Parsed.Info.Extensions {
		if extension.Extension == "tokenMetadata" {
			tokenInfo.Name = extension.State.Name
			tokenInfo.Symbol = extension.State.Symbol

			if len(extension.State.Uri) > 0 {
				publicGateway := "https://ipfs.io/ipfs/"
				if !isURLAccessible(extension.State.Uri) {
					extension.State.Uri = replaceWithPublicGateway(extension.State.Uri, publicGateway)
				}

				ctx, cancelFunc := context.WithTimeout(context.Background(), 3000*time.Millisecond)
				defer cancelFunc()
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, extension.State.Uri, nil)
				if err != nil {
					err = fmt.Errorf("http.NewRequest err:%w", err)
					return token2022Info, tokenInfo, err
				}

				// 执行请求
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					// skip error
					return token2022Info, tokenInfo, nil
				}
				defer func() {
					_ = response.Body.Close()
				}()

				res, err := io.ReadAll(response.Body)

				if err != nil {
					// skip error
					return token2022Info, tokenInfo, nil
				}
				// 检查 Content-Type
				contentType := response.Header.Get("Content-Type")
				if strings.Contains(string(res), "Account has been disabled.") {
					return token2022Info, tokenInfo, nil
				}
				if strings.HasPrefix(contentType, "application/json") {
					tokenUriData, err := transfer.Byte2Struct[TokenUriData](res)
					if err != nil {
						return token2022Info, tokenInfo, nil
					}

					if len(tokenUriData.Website) == 0 {
						tokenUriData.Website = tokenUriData.Extensions.Website
					}
					if len(tokenUriData.Telegram) == 0 {
						tokenUriData.Telegram = tokenUriData.Extensions.Telegram
					}
					if len(tokenUriData.Twitter) == 0 {
						tokenUriData.Twitter = tokenUriData.Extensions.Twitter
					}

					tokenInfo.Uri = tokenUriData
				} else if strings.HasPrefix(contentType, "image/") {
					// maybe picture
					// https://solscan.io/token/2HPtzSqkivqk8P5ySqVxB17b93sXsJN4s77kJp4Eish9#metadata
					// if strings.Contains(err.Error(), "invalid character") {
					// 	tokenInfo.Uri.Image = metaData.Data.Uri
					// }
					// skip error
					tokenInfo.Uri.Image = extension.State.Uri
				} else {

					tokenUriData, err := transfer.Byte2Struct[TokenUriData](res)
					if err != nil {
						err = fmt.Errorf("GetToken2022Info error: %v, url: %v, token address: %v", err, extension.State.Uri, address)
						return token2022Info, tokenInfo, err
					}

					if len(tokenUriData.Website) == 0 {
						tokenUriData.Website = tokenUriData.Extensions.Website
					}
					if len(tokenUriData.Telegram) == 0 {
						tokenUriData.Telegram = tokenUriData.Extensions.Telegram
					}
					if len(tokenUriData.Twitter) == 0 {
						tokenUriData.Twitter = tokenUriData.Extensions.Twitter
					}

					tokenInfo.Uri = tokenUriData
				}

			}
		}
	}

	_ = mintResponse

	return &mintResponse.Parsed.Info, tokenInfo, nil
}
