package contractapi

import (
	"os"
	"testing"
)

func TestContextAndLog(t *testing.T) {
	Reset()
	t.Setenv("VM_BLOCK_HEIGHT", "42")
	t.Setenv("VM_BLOCK_TIME", "1700000000")
	t.Setenv("VM_SENDER", "alice")
	t.Setenv("VM_CONTRACT_ADDRESS", "contract_1")

	if BlockHeight() != 42 {
		t.Fatalf("height=%d", BlockHeight())
	}
	if Sender() != "alice" {
		t.Fatalf("sender=%s", Sender())
	}
	if ContractAddress() != "contract_1" {
		t.Fatalf("contract=%s", ContractAddress())
	}

	Log("Transfer", "from", "a", "to", "b", "amount", 10)
	ev := DrainEvents()
	if len(ev) != 1 || ev[0].Name != "Transfer" {
		t.Fatalf("unexpected events: %+v", ev)
	}
	if ev[0].Fields["amount"] != 10 {
		t.Fatalf("amount=%v", ev[0].Fields["amount"])
	}
	if len(DrainEvents()) != 0 {
		t.Fatal("expected empty after drain")
	}

	_ = os.Unsetenv("VM_BLOCK_HEIGHT")
}
