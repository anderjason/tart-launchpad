package launchpad

import (
	"net"
	"sort"
)

func localLANCIDRs() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var cidrs []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			cidrs = appendPrivateIPv4CIDR(cidrs, seen, *ipNet)
		}
	}
	sort.Strings(cidrs)
	return cidrs
}

func appendPrivateIPv4CIDR(cidrs []string, seen map[string]bool, ipNet net.IPNet) []string {
	ip := ipNet.IP.To4()
	if ip == nil || !ip.IsPrivate() {
		return cidrs
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 || ones == 32 {
		return cidrs
	}
	network := net.IPNet{
		IP:   ip.Mask(ipNet.Mask),
		Mask: ipNet.Mask,
	}
	cidr := network.String()
	if seen[cidr] {
		return cidrs
	}
	seen[cidr] = true
	return append(cidrs, cidr)
}

func parseCIDR(value string) bool {
	if value == "" {
		return false
	}
	ip, ipNet, err := net.ParseCIDR(value)
	return err == nil && ip.To4() != nil && ipNet != nil
}
