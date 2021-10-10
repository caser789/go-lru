package lru

import (
	"testing"
)

func TestLRU(t *testing.T) {
	_, err := New(128)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
}
