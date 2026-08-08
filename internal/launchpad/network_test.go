package launchpad

import (
	"net"
	"reflect"
	"testing"
)

func TestAppendPrivateIPv4CIDRUsesNetworkAddress(t *testing.T) {
	ipNet := net.IPNet{
		IP:   net.ParseIP("192.168.1.42"),
		Mask: net.CIDRMask(24, 32),
	}

	got := appendPrivateIPv4CIDR(nil, map[string]bool{}, ipNet)
	want := []string{"192.168.1.0/24"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CIDRs\n got %#v\nwant %#v", got, want)
	}
}

func TestAppendPrivateIPv4CIDRSkipsPublicLoopbackAndHostRoutes(t *testing.T) {
	seen := map[string]bool{}
	var got []string
	for _, ipNet := range []net.IPNet{
		{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("10.0.0.14"), Mask: net.CIDRMask(32, 32)},
	} {
		got = appendPrivateIPv4CIDR(got, seen, ipNet)
	}

	if len(got) != 0 {
		t.Fatalf("CIDRs = %#v, want none", got)
	}
}

func TestDefaultLANCIDRChoicesAlwaysIncludesFreeText(t *testing.T) {
	choices := defaultLANCIDRChoices()
	if len(choices) == 0 {
		t.Fatal("choices are empty")
	}
	if !choices[len(choices)-1].FreeText {
		t.Fatalf("last choice = %#v, want free text", choices[len(choices)-1])
	}
}

func TestNormalizePrivateIPv4CIDRRejectsInternetWideAndPublicNetworks(t *testing.T) {
	for _, value := range []string{"0.0.0.0/0", "8.8.8.0/24", "10.0.0.0/7"} {
		if _, err := normalizePrivateIPv4CIDR(value); err == nil {
			t.Fatalf("normalizePrivateIPv4CIDR(%q) succeeded; want rejection", value)
		}
	}
}

func TestNormalizePrivateIPv4CIDRRequiresNetworkAddress(t *testing.T) {
	if _, err := normalizePrivateIPv4CIDR("192.168.1.42/24"); err == nil {
		t.Fatal("host-address CIDR succeeded; want rejection")
	}
}

func TestNormalizePrivateIPv4CIDRAcceptsPrivateNetwork(t *testing.T) {
	got, err := normalizePrivateIPv4CIDR("172.16.32.0/20")
	if err != nil {
		t.Fatal(err)
	}
	if got != "172.16.32.0/20" {
		t.Fatalf("CIDR = %q, want canonical private network", got)
	}
}
