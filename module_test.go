package appkit

import (
	"testing"
)

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"demo", "identity", "my-module", "m1"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "Demo", "my_module", "mod.x", "模块"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
	}
}

func TestConfigSpecKeyOrName(t *testing.T) {
	if got := (ConfigSpec{}).KeyOrName("demo"); got != "demo" {
		t.Errorf("default key = %q, want demo", got)
	}
	if got := (ConfigSpec{Key: "custom"}).KeyOrName("demo"); got != "custom" {
		t.Errorf("explicit key = %q, want custom", got)
	}
}
