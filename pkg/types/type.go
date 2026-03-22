package types

type Pair struct {
	ChainId string `json:"chain_id"`
	Addr    string `json:"addr"`

	BaseTokenAddr          string `json:"base_token_addr"`
	TokenAddr              string `json:"token_addr"`
	BaseTokenSymbol        string `json:"base_token_symbol"`
	TokenSymbol            string `json:"token_symbol"`
	BaseTokenDecimal       uint8  `json:"base_token_decimal"`
	TokenDecimal           uint8  `json:"token_decimal"`
	BaseTokenIsNativeToken bool   `json:"base_token_is_native_token"`
	BaseTokenIsToken0      bool   `json:"base_token_is_token_0"`

	TokenTotalSupply    float64 `json:"token_total_supply"`     // 代币总供应量
	InitTokenAmount     float64 `json:"init_token_amount"`      // 初始化代币数量
	InitBaseTokenAmount float64 `json:"init_base_token_amount"` // 初始化基础代币数量

	Name string `json:"name"`

	BlockNum  int64 `json:"block_num"`  // 池子创建Slot
	BlockTime int64 `json:"block_time"` // 池子创建时间

	CurrentBaseTokenAmount float64 `gorm:"column:current_base_token_amount"` // 当前base流动性数量
	CurrentTokenAmount     float64 `gorm:"column:current_token_amount"`      // 当前token流动性数量
}

type PairHotData struct {
	PriceBefore float64 `json:"price_before"`
	PriceNow    float64 `json:"price_now"`
	PriceChange float64 `json:"price_change"`
	UpDown      float64 `json:"up_down"`
	Volume      float64 `gorm:"volume" json:"volume"`
	BuyCount    float64 `gorm:"buy_count" json:"buy_count"`
	SellCount   float64 `gorm:"sell_count" json:"sell_count"`
}
