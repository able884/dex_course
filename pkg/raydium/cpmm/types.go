package cpmm

import (
	"github.com/blocto/solana-go-sdk/common"
	"github.com/near/borsh-go"
)

// SwapEvent 定义了 SwapEvent 事件的结构
type SwapEvent struct {
	PoolID            common.PublicKey `json:"pool_id"`             // 使用 string 来表示 Pubkey（Solana 的公钥）
	InputVaultBefore  uint64           `json:"input_vault_before"`  // 使用 uint64 表示 u64 类型
	OutputVaultBefore uint64           `json:"output_vault_before"` // 使用 uint64 表示 u64 类型
	InputAmount       uint64           `json:"input_amount"`        // 使用 uint64 表示 u64 类型
	OutputAmount      uint64           `json:"output_amount"`       // 使用 uint64 表示 u64 类型
	InputTransferFee  uint64           `json:"input_transfer_fee"`  // 使用 uint64 表示 u64 类型
	OutputTransferFee uint64           `json:"output_transfer_fee"` // 使用 uint64 表示 u64 类型
	BaseInput         bool             `json:"base_input"`          // 使用 bool 表示 bool 类型
}

func DeserializeSwapEvent(data []byte) (result SwapEvent, err error) {
	err = borsh.Deserialize(&result, data)
	return
}

//
// // DeserializeSwapEvent 从字节中反序列化为 SwapEvent
// func DeserializeSwapEvent(data []byte) (*SwapEvent, error) {
// 	if len(data) < 72 {
// 		return nil, fmt.Errorf("数据长度不足")
// 	}
//
// 	var e SwapEvent
// 	reader := bytes.NewReader(data)
//
// 	err := binary.Read(reader, binary.LittleEndian, &e.PoolID)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化失败: %v", err)
// 	}
//
// 	err = binary.Read(reader, binary.LittleEndian, &e.InputVaultBefore)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化输入 Vault 失败: %v", err)
// 	}
//
// 	err = binary.Read(reader, binary.LittleEndian, &e.OutputVaultBefore)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化输出 Vault 失败: %v", err)
// 	}
//
// 	err = binary.Read(reader, binary.LittleEndian, &e.InputAmount)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化输入金额失败: %v", err)
// 	}
//
// 	err = binary.Read(reader, binary.LittleEndian, &e.OutputAmount)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化输出金额失败: %v", err)
// 	}
//
// 	err = binary.Read(reader, binary.LittleEndian, &e.InputTransferFee)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化输入转移费用失败: %v", err)
// 	}
//
// 	err = binary.Read(reader, binary.LittleEndian, &e.OutputTransferFee)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化输出转移费用失败: %v", err)
// 	}
//
// 	// 读取最后一个 bool 值
// 	var baseInputValue byte
// 	err = binary.Read(reader, binary.LittleEndian, &baseInputValue)
// 	if err != nil {
// 		return nil, fmt.Errorf("反序列化 base input 失败: %v", err)
// 	}
// 	e.BaseInput = baseInputValue != 0 // 1 表示 true, 0 表示 false
//
// 	return &e, nil
// }
