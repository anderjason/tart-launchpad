package launchpad

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

var privateIPv4Networks = []*net.IPNet{
	{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
	{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
	{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
}

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

func normalizePrivateIPv4CIDR(value string) (string, error) {
	value = strings.TrimSpace(value)
	ip, network, err := net.ParseCIDR(value)
	if err != nil || ip.To4() == nil || network == nil {
		return "", fmt.Errorf("%w: LAN CIDR must be an IPv4 network: %q", ErrUsage, value)
	}
	networkIP := network.IP.To4()
	if !ip.To4().Equal(networkIP) {
		return "", fmt.Errorf("%w: LAN CIDR must use its network address; use %s", ErrUsage, network.String())
	}
	ones, bits := network.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("%w: LAN CIDR must be an IPv4 network: %q", ErrUsage, value)
	}
	for _, privateNetwork := range privateIPv4Networks {
		privateOnes, _ := privateNetwork.Mask.Size()
		if ones >= privateOnes && privateNetwork.Contains(networkIP) {
			return network.String(), nil
		}
	}
	return "", fmt.Errorf("%w: LAN CIDR must stay within 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16", ErrUsage)
}

func normalizePrivateIPv4CIDRs(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		cidr, err := normalizePrivateIPv4CIDR(value)
		if err != nil {
			return nil, err
		}
		if seen[cidr] {
			continue
		}
		seen[cidr] = true
		result = append(result, cidr)
	}
	return result, nil
}
