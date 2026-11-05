package winbind

import (
	"reflect"
	"testing"
)

var _ Window = (*Recorder)(nil)

func TestRecorderRecordsInOrder(t *testing.T) {
	r := &Recorder{}
	if err := r.Bind("a", func() {}); err != nil {
		t.Fatalf("Bind a: %v", err)
	}
	if err := r.Bind("b", func() {}); err != nil {
		t.Fatalf("Bind b: %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(r.Names, want) {
		t.Fatalf("Names = %v, want %v", r.Names, want)
	}
}

func TestRecorderRejectsDuplicate(t *testing.T) {
	r := &Recorder{}
	if err := r.Bind("a", func() {}); err != nil {
		t.Fatalf("first Bind: %v", err)
	}
	if err := r.Bind("a", func() {}); err == nil {
		t.Fatal("second Bind a returned nil error")
	}
	if want := []string{"a"}; !reflect.DeepEqual(r.Names, want) {
		t.Fatalf("Names = %v, want %v", r.Names, want)
	}
}
