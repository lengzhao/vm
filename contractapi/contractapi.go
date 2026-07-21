// Package contractapi 是合约侧默认库（MVP）：只读链上下文与事件日志。
// 宿主通过环境变量注入上下文；Log 写入进程内缓冲，由 entry 随 JSON 响应回传。
package contractapi

import (
	"os"
	"strconv"
	"sync"
)

// Event 表示合约发出的事件。
type Event struct {
	Name   string         `json:"name"`
	Fields map[string]any `json:"fields"`
}

var (
	mu     sync.Mutex
	events []Event
)

// Reset 清空事件缓冲，每次合约调用入口应调用一次。
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	events = nil
}

// DrainEvents 取出并清空事件缓冲。
func DrainEvents() []Event {
	mu.Lock()
	defer mu.Unlock()
	out := events
	events = nil
	if out == nil {
		return []Event{}
	}
	return out
}

// BlockHeight 返回当前区块高度。
func BlockHeight() uint64 {
	ConsumeGas(GasBlockHeight)
	return envUint64("VM_BLOCK_HEIGHT")
}

// BlockTime 返回当前区块时间戳。
func BlockTime() uint64 {
	ConsumeGas(GasBlockTime)
	return envUint64("VM_BLOCK_TIME")
}

// Sender 返回交易发送方。
func Sender() string {
	ConsumeGas(GasSender)
	return os.Getenv("VM_SENDER")
}

// ContractAddress 返回当前合约地址。
func ContractAddress() string {
	ConsumeGas(GasContractAddr)
	return os.Getenv("VM_CONTRACT_ADDRESS")
}

// Log 记录事件（键值成对传入）。
func Log(eventName string, keyValues ...any) {
	ConsumeGas(GasLog)
	fields := make(map[string]any)
	for i := 0; i+1 < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			continue
		}
		fields[key] = keyValues[i+1]
	}
	mu.Lock()
	defer mu.Unlock()
	events = append(events, Event{Name: eventName, Fields: fields})
}

func envUint64(key string) uint64 {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
