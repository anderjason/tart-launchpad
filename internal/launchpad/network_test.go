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
