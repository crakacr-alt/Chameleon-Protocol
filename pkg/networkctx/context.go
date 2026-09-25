package networkctx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
)

// LinkType is a coarse network type. We intentionally keep it simple,
// because interface names differ between operating systems and devices.
type LinkType string

const (
	LinkUnknown  LinkType = "unknown"
	LinkEthernet LinkType = "ethernet"
	LinkWiFi     LinkType = "wifi"
	LinkMobile   LinkType = "mobile"
	LinkVirtual  LinkType = "virtual"
)

// Context describes the network without saving raw local addresses.
type Context struct {
	ID        string   `json:"id"`
	Link      LinkType `json:"link"`
	IPv4      bool     `json:"ipv4"`
	IPv6      bool     `json:"ipv6"`
	OS        string   `json:"os"`
	Interface string   `json:"interface,omitempty"`
}

// Detect returns a best-effort context for the current machine.
//
// Virtual VPN/tunnel interfaces are intentionally ranked below physical
// interfaces. This helps Chameleon coexist with Tailscale, WireGuard and
// other VPN software instead of learning a route for the tunnel itself.
func Detect() (Context, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return Context{}, fmt.Errorf("list interfaces: %w", err)
	}

	type candidate struct {
		ctx   Context
		score int
		raw   []string
	}
	var candidates []candidate

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		ctx := Context{
			Link:      ClassifyInterface(iface.Name),
			OS:        runtime.GOOS,
			Interface: iface.Name,
		}
		var raw []string
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip == nil || ip.IsLoopback() {
				continue
			}
			if ip.To4() != nil {
				ctx.IPv4 = true
				raw = append(raw, maskedAddress(ip, 24))
			} else {
				ctx.IPv6 = true
				raw = append(raw, maskedAddress(ip, 64))
			}
		}
		if !ctx.IPv4 && !ctx.IPv6 {
			continue
		}

		score := linkScore(ctx.Link)
		if iface.Flags&net.FlagPointToPoint != 0 {
			score -= 30
		}
		candidates = append(candidates, candidate{ctx: ctx, score: score, raw: raw})
	}

	if len(candidates) == 0 {
		return Context{
			ID:   "unknown",
			Link: LinkUnknown,
			OS:   runtime.GOOS,
		}, nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].ctx.Interface < candidates[j].ctx.Interface
		}
		return candidates[i].score > candidates[j].score
	})

	best := candidates[0]
	sort.Strings(best.raw)
	best.ctx.ID = fingerprint(best.ctx, best.raw)
	return best.ctx, nil
}

// ClassifyInterface uses only the interface name and does not require
// platform-specific APIs or root permissions.
func ClassifyInterface(name string) LinkType {
	n := strings.ToLower(name)

	virtual := []string{
		"tailscale", "tun", "tap", "wg", "wireguard", "utun",
		"vpn", "ppp", "docker", "veth", "virbr", "vmnet", "wintun",
	}
	for _, token := range virtual {
		if strings.Contains(n, token) {
			return LinkVirtual
		}
	}

	mobile := []string{"rmnet", "wwan", "cell", "mobile", "pdp", "ccmni"}
	for _, token := range mobile {
		if strings.Contains(n, token) {
			return LinkMobile
		}
	}

	wifi := []string{"wlan", "wifi", "wi-fi", "wl"}
	for _, token := range wifi {
		if strings.Contains(n, token) {
			return LinkWiFi
		}
	}

	ethernet := []string{"eth", "enp", "eno", "ens", "ethernet"}
	for _, token := range ethernet {
		if strings.Contains(n, token) {
			return LinkEthernet
		}
	}

	return LinkUnknown
}

func linkScore(link LinkType) int {
	switch link {
	case LinkEthernet:
		return 100
	case LinkWiFi:
		return 90
	case LinkMobile:
		return 80
	case LinkUnknown:
		return 50
	case LinkVirtual:
		return 10
	default:
		return 0
	}
}

func fingerprint(ctx Context, networkParts []string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(string(ctx.Link)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(ctx.OS))
	for _, part := range networkParts {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(part))
	}
	sum := h.Sum(nil)
	return "net-" + hex.EncodeToString(sum[:8])
}

func maskedAddress(ip net.IP, bits int) string {
	if ip4 := ip.To4(); ip4 != nil {
		mask := net.CIDRMask(bits, 32)
		return ip4.Mask(mask).String() + fmt.Sprintf("/%d", bits)
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return ""
	}
	mask := net.CIDRMask(bits, 128)
	return ip16.Mask(mask).String() + fmt.Sprintf("/%d", bits)
}
