package db

import (
	"reflect"
	"testing"
)

func TestExtractFileNames(t *testing.T) {
	files := []File{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := ExtractFileNames(files)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractFileNames mismatch: got %v want %v", got, want)
	}
}
