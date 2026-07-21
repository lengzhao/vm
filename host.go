package vm

// Address 表示账户或合约地址。
type Address string

// Event 表示合约执行过程中产生的事件。
type Event struct {
	Name   string
	Fields map[string]any
}

// Host 是链宿主提供给 VM 的最小运行时接口。
// MVP 仅覆盖只读上下文与事件日志；Object / Transfer / Call 后置。
type Host interface {
	BlockHeight() uint64
	BlockTime() uint64
	Sender() Address
	ContractAddress() Address
	Log(eventName string, keyValues ...any)
}

// CallContext 描述单次合约调用的宿主上下文。
type CallContext struct {
	Host            Host
	GasLimit        uint64
	Sender          Address
	ContractAddress Address
	BlockHeight     uint64
	BlockTime       uint64
}

// MemoryHost 是用于测试与本地演示的内存版 Host。
type MemoryHost struct {
	Height   uint64
	Time     uint64
	From     Address
	Contract Address
	Events   []Event
}

func (h *MemoryHost) BlockHeight() uint64 { return h.Height }
func (h *MemoryHost) BlockTime() uint64   { return h.Time }
func (h *MemoryHost) Sender() Address     { return h.From }
func (h *MemoryHost) ContractAddress() Address {
	return h.Contract
}

func (h *MemoryHost) Log(eventName string, keyValues ...any) {
	fields := make(map[string]any)
	for i := 0; i+1 < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			continue
		}
		fields[key] = keyValues[i+1]
	}
	h.Events = append(h.Events, Event{Name: eventName, Fields: fields})
}
