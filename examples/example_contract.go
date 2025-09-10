// Package vm 示例智能合约
package vm

// SimpleStorage 合约示例
type SimpleStorage struct {
	Value int `json:"value"`
}

// NewSimpleStorage 创建新的存储合约
func NewSimpleStorage() *SimpleStorage {
	return &SimpleStorage{
		Value: 0,
	}
}

// Set 设置值
func (s *SimpleStorage) Set(value int) {
	s.Value = value
}

// Get 获取值
func (s *SimpleStorage) Get() int {
	return s.Value
}

// Add 将值增加指定数量
func (s *SimpleStorage) Add(amount int) int {
	s.Value += amount
	return s.Value
}

// Subtract 将值减少指定数量
func (s *SimpleStorage) Subtract(amount int) int {
	s.Value -= amount
	return s.Value
}

// Reset 重置值为0
func (s *SimpleStorage) Reset() {
	s.Value = 0
}
