package flow

import "testing"

func TestParseUintSlice(t *testing.T) {
	got, err := ParseUintSlice([]string{"1", "2", "30"})
	if err != nil {
		t.Fatalf("ParseUintSlice: %v", err)
	}
	want := []uint{1, 2, 30}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestParseUintSlice_ErrorsOnGarbage(t *testing.T) {
	if _, err := ParseUintSlice([]string{"not-a-number"}); err == nil {
		t.Fatal("expected an error for a non-numeric id")
	}
}
