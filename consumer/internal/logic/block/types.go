package block

/*
TokenAccount
1.	Owner (string)：表示账户的所有者的地址（通常是钱包的公钥）。
2.	TokenAccountAddress (string)：该代币账户的地址，在 Solana 网络中每个账户都有唯一的地址。
3.	TokenAddress (string)：表示此账户中存储的代币的地址（例如 USDT、SOL 或其他代币的智能合约地址）。
4.	TokenDecimal (uint8)：表示代币的小数位数。Solana 中的代币通常可以有不同的小数位数，这个字段用于存储该代币的精度。
5.	PreValue (int64)：账户在某个时间点之前的余额，通常是代币的余额变动之前的值。
6.	PostValue (int64)：账户在某个时间点之后的余额，即变动后的余额。
7.	Closed (bool)：标记该代币账户是否已关闭。
8.	Init (bool)：标记该代币账户是否已初始化，表示账户是否已在 Solana 网络上创建。
9.	PostValueUIString (string)：格式化后的 PostValue 字段，通常用于前端展示给用户。可能包含 PostValue 的单位转换（如从整数转换为小数，考虑到 TokenDecimal）。
10.	PreValueUIString (string)：格式化后的 PreValue 字段，同样用于前端显示。
*/
type TokenAccount struct {
	Owner               string
	TokenAccountAddress string
	TokenAddress        string
	TokenDecimal        uint8
	PreValue            int64
	PostValue           int64
	Closed              bool
	Init                bool
	PostValueUIString   string
	PreValueUIString    string
}
