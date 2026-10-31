package env

import (
	"slices"
	"testing"
	"time"
)

func TestHelpers(t *testing.T) {
	t.Setenv("E_STR", "x")
	t.Setenv("E_INT", "7")
	t.Setenv("E_INT_BAD", "sete")
	t.Setenv("E_BOOL", "true")
	t.Setenv("E_BOOL_BAD", "talvez")
	t.Setenv("E_DUR", "90s")
	t.Setenv("E_DUR_BAD", "logo")
	t.Setenv("E_LIST", " https://a.com, ,https://b.com ")

	if GetOrDefault("E_STR", "d") != "x" || GetOrDefault("E_NONE", "d") != "d" {
		t.Error("GetOrDefault")
	}
	if GetIntOrDefault("E_INT", 1) != 7 || GetIntOrDefault("E_INT_BAD", 1) != 1 || GetIntOrDefault("E_NONE", 1) != 1 {
		t.Error("GetIntOrDefault")
	}
	if !GetBoolOrDefault("E_BOOL", false) || GetBoolOrDefault("E_BOOL_BAD", false) || !GetBoolOrDefault("E_NONE", true) {
		t.Error("GetBoolOrDefault")
	}
	if GetDurationOrDefault("E_DUR", 0) != 90*time.Second || GetDurationOrDefault("E_DUR_BAD", time.Second) != time.Second {
		t.Error("GetDurationOrDefault")
	}
	if got := GetListOrDefault("E_LIST", nil); !slices.Equal(got, []string{"https://a.com", "https://b.com"}) {
		t.Errorf("GetListOrDefault = %v", got)
	}
	if got := GetListOrDefault("E_NONE", []string{"d"}); !slices.Equal(got, []string{"d"}) {
		t.Error("GetListOrDefault fallback")
	}
}
