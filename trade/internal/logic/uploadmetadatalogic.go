package logic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	lighthouse "github.com/lighthouse-web3/lighthouse-go-sdk/lighthouse"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// UploadMetadataLogic 处理代币元数据上传业务逻辑
type UploadMetadataLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewUploadMetadataLogic 创建代币元数据上传逻辑处理器
func NewUploadMetadataLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadMetadataLogic {
	return &UploadMetadataLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// UploadTokenMetadata 上传代币元数据到 IPFS
// 该方法负责：
// 1. 参数校验（API Key、必填字段）
// 2. 如果提供了图片内容（base64），先上传图片到 IPFS
// 3. 构建标准代币元数据 JSON
// 4. 上传元数据 JSON 到 IPFS
// 5. 返回 IPFS CID、URI 和访问链接
func (l *UploadMetadataLogic) UploadTokenMetadata(in *trade.UploadMetadataRequest) (*trade.UploadMetadataResponse, error) {
	// 1. 校验 IPFS API Key 是否配置
	if l.svcCtx.NFTStorageKey == "" {
		return nil, fmt.Errorf("IPFS API Key not configured")
	}

	// 2. 校验必填字段（代币名称和符号）
	if in.Name == "" || in.Symbol == "" {
		return nil, fmt.Errorf("name/symbol required")
	}

	// 3. 如果未提供图片URL但提供了base64图片内容，先上传图片
	imageURI := in.ImageUri
	if imageURI == "" && in.ImageContentBase64 != "" {
		// 解码 base64 图片数据
		imgData, err := base64.StdEncoding.DecodeString(in.ImageContentBase64)
		if err != nil {
			return nil, fmt.Errorf("invalid image_content_base64: %w", err)
		}

		// 根据配置的提供商上传图片
		provider := strings.ToLower(l.svcCtx.Config.Ipfs.Provider)
		var cid string
		switch provider {
		case "lighthouse":
			// 优先使用 SDK，失败降级到 HTTP
			if l.svcCtx.LhClient != nil {
				reader := bytes.NewReader(imgData)
				res, err := l.svcCtx.LhClient.Storage().UploadReader(l.ctx, "image.jpg", int64(len(imgData)), reader)
				if err != nil {
					c2, err2 := uploadViaLighthouse(l.ctx, l.svcCtx.NFTStorageKey, "image.jpg", imgData)
					if err2 != nil {
						return nil, fmt.Errorf("lighthouse image upload failed: %v; fallback err: %v", err, err2)
					}
					cid = c2
				} else {
					cid = res.Hash
				}
			} else {
				c2, err2 := uploadViaLighthouse(l.ctx, l.svcCtx.NFTStorageKey, "image.jpg", imgData)
				if err2 != nil {
					return nil, err2
				}
				cid = c2
			}
		default:
			// 默认使用 NFT.Storage
			c2, err2 := uploadViaNFTStorageBinary(l.ctx, l.svcCtx.NFTStorageKey, imgData)
			if err2 != nil {
				return nil, err2
			}
			cid = c2
		}

		// 构建图片的 IPFS URI
		if cid != "" {
			imageURI = "ipfs://" + cid
		}
	}

	// 4. 构建代币元数据 JSON（兼容标准 NFT 元数据格式）
	meta := map[string]interface{}{
		"name":        in.Name,           // 代币名称
		"symbol":      in.Symbol,         // 代币符号
		"description": in.Description,    // 代币描述
		"image":       imageURI,          // 图片 URI
		"website":     in.Website,        // 官网链接
		"twitter":     in.Twitter,        // Twitter 链接
		"telegram":    in.Telegram,       // Telegram 链接
		"extensions": map[string]string{  // 扩展字段（便于索引）
			"website":  in.Website,
			"twitter":  in.Twitter,
			"telegram": in.Telegram,
		},
	}

	// 5. 上传元数据 JSON 到 IPFS
	provider := strings.ToLower(l.svcCtx.Config.Ipfs.Provider)
	var cid string
	switch provider {
	case "lighthouse":
		if l.svcCtx.LhClient != nil {
			v, err := uploadMetadataViaLighthouseSDK(l.ctx, l.svcCtx.LhClient, meta)
			if err != nil {
				// 降级到 HTTP
				v2, err2 := uploadMetadataViaLighthouse(l.ctx, l.svcCtx.NFTStorageKey, meta)
				if err2 != nil {
					return nil, fmt.Errorf("lighthouse upload failed: %v; fallback err: %v", err, err2)
				}
				cid = v2
			} else {
				cid = v
			}
		} else {
			v, err := uploadMetadataViaLighthouse(l.ctx, l.svcCtx.NFTStorageKey, meta)
			if err != nil {
				return nil, err
			}
			cid = v
		}
	default:
		v, err := uploadMetadataViaNFTStorage(l.ctx, l.svcCtx.NFTStorageKey, meta)
		if err != nil {
			return nil, err
		}
		cid = v
	}

	// 6. 构建返回结果
	uri := "ipfs://" + cid
	gw := l.svcCtx.IpfsGateway
	if gw == "" {
		gw = "https://ipfs.io/ipfs/"
	}
	url := gw + cid

	return &trade.UploadMetadataResponse{Cid: cid, Uri: uri, Url: url, ImageUri: imageURI}, nil
}

// uploadMetadataViaNFTStorage 通过 NFT.Storage 上传元数据
func uploadMetadataViaNFTStorage(ctx context.Context, apiKey string, meta map[string]interface{}) (string, error) {
	// 序列化元数据为 JSON
	body, _ := json.Marshal(meta)

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.nft.storage/upload", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("nft.storage http status: %d, body: %s", resp.StatusCode, string(b))
	}

	// 解析响应中的 CID
	var s1 struct {
		Ok    bool `json:"ok"`
		Value struct {
			Cid string `json:"cid"`
		} `json:"value"`
	}
	if err := json.Unmarshal(b, &s1); err == nil && s1.Value.Cid != "" {
		return s1.Value.Cid, nil
	}

	// 尝试其他响应格式
	var m map[string]any
	if err := json.Unmarshal(b, &m); err == nil {
		if v, ok := m["cid"].(string); ok && v != "" {
			return v, nil
		}
		if v, ok := m["value"].(map[string]any); ok {
			if c, ok := v["cid"].(string); ok && c != "" {
				return c, nil
			}
		}
	}

	return "", fmt.Errorf("nft.storage response missing cid")
}

// uploadMetadataViaLighthouse 通过 Lighthouse HTTP API 上传元数据
func uploadMetadataViaLighthouse(ctx context.Context, apiKey string, meta map[string]interface{}) (string, error) {
	// 序列化元数据为 JSON
	content, _ := json.Marshal(meta)

	// 创建 multipart form
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "metadata.json")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(content); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	// Lighthouse IPFS 上传端点
	url := "https://node.lighthouse.storage/api/v0/add"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}

	// Lighthouse 使用 Authorization: <API_KEY>（不带 Bearer）
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	// 发送请求
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	b, _ := io.ReadAll(resp.Body)

	// 处理认证错误，尝试 X-API-Key 头
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_ = resp.Body.Close()
		req2, err2 := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf.Bytes()))
		if err2 != nil {
			return "", err2
		}
		req2.Header.Set("X-API-Key", apiKey)
		req2.Header.Set("Content-Type", mw.FormDataContentType())

		resp2, err2 := http.DefaultClient.Do(req2)
		if err2 != nil {
			return "", err2
		}
		defer resp2.Body.Close()

		b, _ = io.ReadAll(resp2.Body)
		if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
			return "", fmt.Errorf("lighthouse http status: %d, body: %s", resp2.StatusCode, string(b))
		}
	} else {
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("lighthouse http status: %d, body: %s", resp.StatusCode, string(b))
		}
	}

	// 解析响应中的 Hash
	var s1 struct {
		Data struct {
			Hash string `json:"Hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &s1); err == nil && s1.Data.Hash != "" {
		return s1.Data.Hash, nil
	}

	var s2 struct {
		Hash string `json:"Hash"`
	}
	if err := json.Unmarshal(b, &s2); err == nil && s2.Hash != "" {
		return s2.Hash, nil
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err == nil {
		if v, ok := m["Hash"].(string); ok && v != "" {
			return v, nil
		}
		if d, ok := m["data"].(map[string]any); ok {
			if h, ok := d["Hash"].(string); ok && h != "" {
				return h, nil
			}
		}
	}

	return "", fmt.Errorf("lighthouse response missing cid: %s", string(b))
}

// uploadMetadataViaLighthouseSDK 通过 Lighthouse Go SDK 上传元数据
func uploadMetadataViaLighthouseSDK(ctx context.Context, client *lighthouse.Client, meta map[string]interface{}) (string, error) {
	content, _ := json.Marshal(meta)
	reader := bytes.NewReader(content)

	// 调用 SDK 上传
	res, err := client.Storage().UploadReader(ctx, "metadata.json", int64(len(content)), reader)
	if err != nil {
		return "", err
	}

	if res == nil || res.Hash == "" {
		return "", fmt.Errorf("lighthouse sdk returned empty hash")
	}

	return res.Hash, nil
}
