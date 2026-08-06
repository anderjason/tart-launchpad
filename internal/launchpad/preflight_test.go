package launchpad

import "testing"

func TestPlanUsesSoftnet(t *testing.T) {
	plan := Plan{Steps: []CommandStep{{
		Args: []string{"tart", "run", "--net-softnet-block=0.0.0.0/0", "dev"},
	}}}
	if !planUsesSoftnet(plan) {
		t.Fatal("expected plan to use Softnet")
	}
}

func TestPlanUsesSoftnetIgnoresHostMode(t *testing.T) {
	plan := Plan{Steps: []CommandStep{{
		Args: []string{"tart", "run", "--net-host", "dev"},
	}}}
	if planUsesSoftnet(plan) {
		t.Fatal("host-only mode should not require Softnet")
	}
}

func TestHasFlagPrefix(t *testing.T) {
	if !hasFlagPrefix("--net-softnet-allow=192.168.1.0/24", "--net-softnet-allow=") {
		t.Fatal("expected prefix match")
	}
	if hasFlagPrefix("--net-softnet-allow", "--net-softnet-allow=") {
		t.Fatal("expected missing equals sign not to match")
	}
}
