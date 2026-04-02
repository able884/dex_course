package constants

type ChainId int

const (
	ETH  ChainId = 1
	BSC  ChainId = 56
	Base ChainId = 8453
	Sol  ChainId = 100000
	//Trx  ChainId = 110000
)

// var ChainIds = []ChainId{ETH, BSC, Base, Sol, Trx}
var ChainIds = []ChainId{Sol, BSC, ETH, Base}
