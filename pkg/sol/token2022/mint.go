package token2022

type MintResponse struct {
	Parsed  Parsed `json:"parsed"`
	Program string `json:"program"`
	Space   int    `json:"space"`
}

type Parsed struct {
	Info Info   `json:"info"`
	Type string `json:"type"`
}

type Info struct {
	Decimals        int         `json:"decimals"`
	Extensions      []Extension `json:"extensions"`
	FreezeAuthority *string     `json:"freezeAuthority"` // 使用指针以处理可能为 null 的情况
	IsInitialized   bool        `json:"isInitialized"`
	MintAuthority   string      `json:"mintAuthority"`
	Supply          string      `json:"supply"`
}

type Extension struct {
	Extension string `json:"extension"`
	State     State  `json:"state"`
}

type State struct {
	Authority       *string `json:"authority"` // 使用指针以处理可能为 null 的情况
	MetadataAddress string  `json:"metadataAddress,omitempty"`
	// AdditionalMetadata []string `json:"additionalMetadata"`
	Mint            string `json:"mint"`
	Name            string `json:"name"`
	Symbol          string `json:"symbol"`
	UpdateAuthority string `json:"updateAuthority"`
	Uri             string `json:"uri"`
}
