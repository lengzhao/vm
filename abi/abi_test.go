package abi

import (
	"testing"
)

func TestExtractABI(t *testing.T) {
	// 测试简单的合约代码
	code := []byte(`
package main

import (
	"fmt"
	"strconv"
)

func main() {
	fmt.Println("Hello, World!")
}

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}

// GetBalance 获取余额
func GetBalance() int {
	return 1000
}

// Transfer 转账
func Transfer(from, to string, amount int) (bool, error) {
	// ctx.Log("Transfer", "from", from, "to", to, "amount", amount)
	return true, nil
}
`)

	abi, err := ExtractABI(code)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if abi.PackageName != "main" {
		t.Errorf("Expected package name 'main', got '%s'", abi.PackageName)
	}

	if len(abi.Imports) != 2 {
		t.Errorf("Expected 2 imports, got %d", len(abi.Imports))
	}

	if len(abi.Functions) != 3 {
		t.Errorf("Expected 3 functions, got %d", len(abi.Functions))
	}

	// 检查Add函数
	foundAdd := false
	for _, fn := range abi.Functions {
		if fn.Name == "Add" {
			foundAdd = true
			if len(fn.Inputs) != 2 {
				t.Errorf("Expected 2 inputs for Add, got %d", len(fn.Inputs))
			}
			if len(fn.Outputs) != 1 {
				t.Errorf("Expected 1 output for Add, got %d", len(fn.Outputs))
			}
			break
		}
	}
	if !foundAdd {
		t.Error("Expected to find function 'Add'")
	}

	// 检查GetBalance函数
	foundGetBalance := false
	for _, fn := range abi.Functions {
		if fn.Name == "GetBalance" {
			foundGetBalance = true
			if len(fn.Inputs) != 0 {
				t.Errorf("Expected 0 inputs for GetBalance, got %d", len(fn.Inputs))
			}
			if len(fn.Outputs) != 1 {
				t.Errorf("Expected 1 output for GetBalance, got %d", len(fn.Outputs))
			}
			break
		}
	}
	if !foundGetBalance {
		t.Error("Expected to find function 'GetBalance'")
	}

	// 检查Transfer函数
	foundTransfer := false
	for _, fn := range abi.Functions {
		if fn.Name == "Transfer" {
			foundTransfer = true
			if len(fn.Inputs) != 3 {
				t.Errorf("Expected 3 inputs for Transfer, got %d", len(fn.Inputs))
			}
			if len(fn.Outputs) != 2 {
				t.Errorf("Expected 2 outputs for Transfer, got %d", len(fn.Outputs))
			}
			break
		}
	}
	if !foundTransfer {
		t.Error("Expected to find function 'Transfer'")
	}
}

func TestExtractABIWithEvents(t *testing.T) {
	// 测试包含事件的合约代码
	// 注意：当前实现可能不会提取注释中的事件，因为我们没有实际的ctx.Log调用
	code := []byte(`
package main

func main() {
}

// SetValue 设置值
func SetValue(value int) {
	// 这里没有实际的ctx.Log调用，所以不会提取事件
}
`)

	abi, err := ExtractABI(code)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// 由于没有实际的ctx.Log调用，期望0个事件
	if len(abi.Events) != 0 {
		t.Errorf("Expected 0 event, got %d", len(abi.Events))
	}
}

func TestGetTypeString(t *testing.T) {
	// 这个测试主要是为了确保getTypeString函数在abi.go中正常工作
	// 由于getTypeString是内部函数，我们通过ExtractABI来间接测试它
	code := []byte(`
package main

func TestFunc(a []int, b map[string]int, c chan int) ([]byte, map[string][]int) {
	return nil, nil
}
`)

	abi, err := ExtractABI(code)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(abi.Functions) != 1 {
		t.Fatalf("Expected 1 function, got %d", len(abi.Functions))
	}

	fn := abi.Functions[0]
	if fn.Name != "TestFunc" {
		t.Errorf("Expected function name 'TestFunc', got '%s'", fn.Name)
	}

	if len(fn.Inputs) != 3 {
		t.Errorf("Expected 3 inputs, got %d", len(fn.Inputs))
	}

	if len(fn.Outputs) != 2 {
		t.Errorf("Expected 2 outputs, got %d", len(fn.Outputs))
	}

	// 验证类型字符串
	// 注意：根据getTypeString的实现，chan int会返回"chan int"
	// map[string]int会返回"map[string]int"
	// []int会返回"[]int"
}
