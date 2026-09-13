//go:build !no_easytier

package outbound

import (
	"strings"
	"testing"

	"github.com/metacubex/mihomo/component/easytier"
)

func TestValidateEasyTierFFIOption(t *testing.T) {
	err := validateEasyTierFFIOption(EasyTierOption{
		Name:         "et",
		FFILibrary:   "libeasytier_ffi.so",
		InstanceName: "mihomo-easytier",
		Config:       "instance_name = \"mihomo-easytier\"\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := validateEasyTierFFIOption(EasyTierOption{FFILibrary: "x"}); err == nil {
		t.Fatal("expected missing instance-name")
	}
	if err := validateEasyTierFFIOption(EasyTierOption{
		FFILibrary:   "x",
		InstanceName: "n",
		Config:       "a",
		ConfigFile:   "b",
	}); err == nil {
		t.Fatal("expected config xor config-file")
	}
	if err := validateEasyTierFFIOption(EasyTierOption{
		FFILibrary:   "x",
		InstanceName: "n",
	}); err == nil {
		t.Fatal("expected missing config")
	}
}

func TestEnsureTOMLInstanceName(t *testing.T) {
	got := ensureTOMLInstanceName("[network_identity]\nnetwork_name = \"n\"\n", "mihomo-easytier")
	if !strings.Contains(got, `instance_name = "mihomo-easytier"`) {
		t.Fatalf("missing instance_name:\n%s", got)
	}
	kept := ensureTOMLInstanceName("inst_name = \"already\"\n", "other")
	if strings.Contains(kept, "instance_name") {
		t.Fatalf("should keep existing inst_name:\n%s", kept)
	}
}

func TestFFIConfigForcesNoTun(t *testing.T) {
	raw := ensureTOMLInstanceName("[network_identity]\nnetwork_name = \"example\"\nnetwork_secret = \"CHANGE_ME\"\n", "mihomo-easytier")
	got := easytier.ApplyRequiredFlags(raw)
	if !strings.Contains(got, "no_tun = true") {
		t.Fatalf("missing no_tun:\n%s", got)
	}
	if !strings.Contains(got, "bind_device = false") {
		t.Fatalf("missing bind_device:\n%s", got)
	}
}
