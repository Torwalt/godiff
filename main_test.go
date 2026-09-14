package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Torwalt/godiff/internal/gitx"
)

func TestPositionalComparison(t *testing.T) {
	exclude := []string{":(exclude)vendor"}
	cmp, ok, err := positionalComparison([]string{"636c8908c"}, exclude)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if cmp.Kind != gitx.KindShow || cmp.ID != "show:636c8908c" ||
		!reflect.DeepEqual(cmp.Args, []string{"636c8908c"}) ||
		!reflect.DeepEqual(cmp.Exclude, exclude) {
		t.Errorf("comparison = %+v", cmp)
	}

	exclude[0] = "changed"
	if cmp.Exclude[0] != ":(exclude)vendor" {
		t.Errorf("comparison aliases exclusions: %v", cmp.Exclude)
	}
}

func TestPositionalComparisonBounds(t *testing.T) {
	if _, ok, err := positionalComparison(nil, nil); err != nil || ok {
		t.Errorf("empty args: ok=%v err=%v", ok, err)
	}
	if _, _, err := positionalComparison([]string{"one", "two"}, nil); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Errorf("two args error = %v", err)
	}
}
