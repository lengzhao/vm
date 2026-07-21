package contractapi

import "sync"

// Gas 表常量（本阶段生效项）。
const (
	GasEntryBase    = 10
	GasFuncOrLoop   = 1
	GasBlockHeight  = 1
	GasBlockTime    = 1
	GasSender       = 1
	GasContractAddr = 1
	GasLog          = 2
)

var (
	gasMu    sync.Mutex
	gasLimit uint64
	gasUsed  uint64
)

// InitGas 设置 gas 上限并清零已用量。
func InitGas(limit uint64) {
	gasMu.Lock()
	defer gasMu.Unlock()
	gasLimit = limit
	gasUsed = 0
}

// ConsumeGas 增加已用 gas；limit>0 且超限时 panic。
func ConsumeGas(amount uint64) {
	gasMu.Lock()
	defer gasMu.Unlock()
	if gasLimit > 0 && gasUsed+amount > gasLimit {
		panic("gas limit exceeded")
	}
	gasUsed += amount
}

// GasUsed 返回当前已消耗 gas。
func GasUsed() uint64 {
	gasMu.Lock()
	defer gasMu.Unlock()
	return gasUsed
}

// ResetGas 清零已用 gas 与上限。
func ResetGas() {
	gasMu.Lock()
	defer gasMu.Unlock()
	gasLimit = 0
	gasUsed = 0
}
