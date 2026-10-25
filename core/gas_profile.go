package core

// FunctionGasProfile 单个导出函数的静态 Gas 特征。
type FunctionGasProfile struct {
	FuncEntries int            `json:"func_entries"`
	LoopSites   int            `json:"loop_sites"`
	APICalls    map[string]int `json:"api_calls"`
}

// GasProfile 合约源码扫描得到的静态 Gas 画像。
type GasProfile struct {
	Version   int                           `json:"version"`
	EntryGas  uint64                        `json:"entry_gas"`
	Functions map[string]FunctionGasProfile `json:"functions"`
}
