package smsquota_test

import (
	"testing"
	"time"

	"github.com/kymjs/noteapi/internal/smsquota"
)

func TestWindowBeginSendLimit(t *testing.T) {
	w := smsquota.New(3, time.Hour)
	keys := []string{"k1", "k2"}
	ok, undo := w.BeginSend(keys)
	if !ok || undo == nil {
		t.Fatal("first send should succeed")
	}
	ok2, _ := w.BeginSend(keys)
	if !ok2 {
		t.Fatal("second send should succeed")
	}
	ok3, _ := w.BeginSend(keys)
	if !ok3 {
		t.Fatal("third send should succeed")
	}
	ok4, _ := w.BeginSend(keys)
	if ok4 {
		t.Fatal("fourth send should fail")
	}
	undo()
	ok5, _ := w.BeginSend(keys)
	if !ok5 {
		t.Fatal("after undo one slot should be free")
	}
}
