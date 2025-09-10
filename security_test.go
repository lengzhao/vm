package vm

import (
	"testing"
)

func TestNewSecurityReviewer(t *testing.T) {
	reviewer := NewSecurityReviewer()
	
	if reviewer == nil {
		t.Error("Expected SecurityReviewer to be created, got nil")
	}
}

func TestIsKeywordAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()
	
	// 测试允许的关键字
	allowedKeywords := []string{"int", "string", "if", "for", "func", "package"}
	for _, keyword := range allowedKeywords {
		if !reviewer.IsKeywordAllowed(keyword) {
			t.Errorf("Expected keyword '%s' to be allowed", keyword)
		}
	}
	
	// 测试不允许的关键字
	disallowedKeywords := []string{"unsafe", "go", "select", "chan", "goto", "map", "cap"}
	for _, keyword := range disallowedKeywords {
		if reviewer.IsKeywordAllowed(keyword) {
			t.Errorf("Expected keyword '%s' to be disallowed", keyword)
		}
	}
}

func TestIsImportAllowed(t *testing.T) {
	reviewer := NewSecurityReviewer()
	
	// 测试允许的导入
	allowedImports := []string{"fmt", "strconv", "math", "time", "errors", "github.com/lengzhao/vm"}
	for _, imp := range allowedImports {
		if !reviewer.IsImportAllowed(imp) {
			t.Errorf("Expected import '%s' to be allowed", imp)
		}
	}
	
	// 测试不允许的导入
	disallowedImports := []string{"os", "net", "syscall", "unsafe"}
	for _, imp := range disallowedImports {
		if reviewer.IsImportAllowed(imp) {
			t.Errorf("Expected import '%s' to be disallowed", imp)
		}
	}
}

func TestReview_ValidCode(t *testing.T) {
	reviewer := NewSecurityReviewer()
	
	validCode := `
package main

import (
	"fmt"
	"strconv"
)

func main() {
	fmt.Println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}
`
	
	err := reviewer.Review(validCode)
	if err != nil {
		t.Errorf("Expected no error for valid code, got %v", err)
	}
}

func TestReview_InvalidImport(t *testing.T) {
	reviewer := NewSecurityReviewer()
	
	invalidCode := `
package main

import (
	"os"
)

func main() {
	fmt.Println("Hello, World!")
}
`
	
	err := reviewer.Review(invalidCode)
	if err == nil {
		t.Error("Expected error for invalid import, got nil")
	}
	
	secErr, ok := err.(*SecurityError)
	if !ok {
		t.Errorf("Expected SecurityError, got %T", err)
	}
	
	if secErr.ErrorType != ImportNotAllowed {
		t.Errorf("Expected ImportNotAllowed error type, got %v", secErr.ErrorType)
	}
}