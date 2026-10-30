package vm

import "testing"

func TestMemoryHost(t *testing.T) {
	h := &MemoryHost{
		Height:   10,
		Time:     1700000000,
		From:     "sender",
		Contract: "contract",
	}

	if h.BlockHeight() != 10 {
		t.Fatalf("unexpected height: %d", h.BlockHeight())
	}
	if h.Sender() != "sender" {
		t.Fatalf("unexpected sender: %s", h.Sender())
	}

	h.Log("Transfer", "from", "a", "to", "b", "amount", 1)
	if len(h.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(h.Events))
	}
	if h.Events[0].Name != "Transfer" {
		t.Fatalf("unexpected event name: %s", h.Events[0].Name)
	}
	if h.Events[0].Fields["amount"] != 1 {
		t.Fatalf("unexpected amount field: %v", h.Events[0].Fields["amount"])
	}
}
