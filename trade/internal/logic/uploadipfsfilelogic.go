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

// UploadIpfsFileLogic 处理 IPFS 文件上传业务逻辑
type UploadIpfsFileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewUploadIpfsFileLogic 创建 IPFS 文件上传逻辑处理器
func NewUploadIpfsFileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadIpfsFileLogic {
	return &UploadIpfsFileLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// UploadIpfsFile 上传文件到 IPFS
// 该方法负责：
// 1. 参数校验
// 2. 解码 base64 内容
// 3. 根据配置选择上传提供商
// 4. 返回 IPFS CID 和访问链接
func (l *UploadIpfsFileLogic) UploadIpfsFile(in *trade.UploadIpfsFileRequest) (*trade.UploadIpfsFileResponse, error) {
	// 1. 校验 API Key
	if l.svcCtx.NFTStorageKey == "" {
		return nil, fmt.Errorf("IPFS API Key 未配置")
	}

	// 2. 校验内容
	if in.ContentBase64 == "" {
		return nil, fmt.Errorf("content_base64 为空")
	}

	// 3. 解码 base64
	data, err := base64.StdEncoding.DecodeString(in.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}

	// 4. 根据提供商选择上传方式
	provider := strings.ToLower(l.svcCtx.Config.Ipfs.Provider)
	var cid string
	switch provider {
	case "lighthouse":
		if l.svcCtx.LhClient != nil {
			v, err := uploadViaLighthouseSDK(l.ctx, l.svcCtx.LhClient, in.Filename, data)
			if err != nil {
				// fallback to HTTP
				v2, err2 := uploadViaLighthouse(l.ctx, l.svcCtx.NFTStorageKey, in.Filename, data)
				if err2 != nil {
					return nil, fmt.Errorf("lighthouse upload failed: %v; fallback err: %v", err, err2)
				}
				cid = v2
			} else {
				cid = v
			}
		} else {
			v, err := uploadViaLighthouse(l.ctx, l.svcCtx.NFTStorageKey, in.Filename, data)
			if err != nil {
				return nil, err
			}
			cid = v
		}
	default:
		v, err := uploadViaNFTStorageBinary(l.ctx, l.svcCtx.NFTStorageKey, data)
		if err != nil {
			return nil, err
		}
		cid = v
	}

	if cid == "" {
		return nil, fmt.Errorf("上传失败")
	}

	// 5. 构建返回结果
	uri := "ipfs://" + cid
	gw := l.svcCtx.IpfsGateway
	if gw == "" {
		gw = "https://ipfs.io/ipfs/"
	}
	url := gw + cid

	return &trade.UploadIpfsFileResponse{Cid: cid, Uri: uri, Url: url}, nil
}

// uploadViaNFTStorageBinary 通过 NFT.Storage 二进制上传
func uploadViaNFTStorageBinary(ctx context.Context, apiKey string, data []byte) (string, error) {
	logx.Infof("使用 NFT.Storage 二进制上传，数据大小: %d bytes", len(data))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.nft.storage/upload", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("nft.storage HTTP 状态: %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var s1 struct {
		Ok    bool `json:"ok"`
		Value struct {
			Cid string `json:"cid"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &s1); err == nil && s1.Value.Cid != "" {
		return s1.Value.Cid, nil
	}

	// 尝试其他响应格式
	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		if v, ok := m["cid"].(string); ok && v != "" {
			return v, nil
		}
		if v, ok := m["value"].(map[string]any); ok {
			if c, ok := v["cid"].(string); ok && c != "" {
				return c, nil
			}
		}
	}

	return "", fmt.Errorf("nft.storage 响应缺少 cid")
}

// uploadViaLighthouse 通过 Lighthouse HTTP API 上传
func uploadViaLighthouse(ctx context.Context, apiKey string, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// 创建表单文件
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	// 发送请求
	url := "https://node.lighthouse.storage/api/v0/add"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}

	// Lighthouse 使用 Authorization: <API_KEY>（不带 Bearer）
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	body, _ := io.ReadAll(resp.Body)

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

		body, _ = io.ReadAll(resp2.Body)
		if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
			return "", fmt.Errorf("lighthouse HTTP 状态: %d, body: %s", resp2.StatusCode, string(body))
		}
	} else {
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("lighthouse HTTP 状态: %d, body: %s", resp.StatusCode, string(body))
		}
	}

	// 解析响应
	var s1 struct {
		Data struct {
			Hash string `json:"Hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &s1); err == nil && s1.Data.Hash != "" {
		return s1.Data.Hash, nil
	}

	var s2 struct {
		Hash string `json:"Hash"`
	}
	if err := json.Unmarshal(body, &s2); err == nil && s2.Hash != "" {
		return s2.Hash, nil
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		if v, ok := m["Hash"].(string); ok && v != "" {
			return v, nil
		}
		if d, ok := m["data"].(map[string]any); ok {
			if h, ok := d["Hash"].(string); ok && h != "" {
				return h, nil
			}
		}
	}

	return "", fmt.Errorf("lighthouse 响应缺少 cid: %s", string(body))
}

// uploadViaLighthouseSDK 通过 Lighthouse Go SDK 上传
func uploadViaLighthouseSDK(ctx context.Context, client *lighthouse.Client, filename string, data []byte) (string, error) {
	reader := bytes.NewReader(data)
	res, err := client.Storage().UploadReader(ctx, filename, int64(len(data)), reader)
	if err != nil {
		return "", err
	}
	if res == nil || res.Hash == "" {
		return "", fmt.Errorf("lighthouse SDK 返回空 hash")
	}
	return res.Hash, nil
}
