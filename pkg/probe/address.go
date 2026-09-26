package probe

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

// IPFamily identifies the address family that actually won a connection race.
type IPFamily string

const (
	FamilyIPv4 IPFamily = "ipv4"
	FamilyIPv6 IPFamily = "ipv6"
)

// IPResolver is intentionally tiny so address ordering can be tested without
// depending on the machine running the tests.
type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// ResolvedAddress is one concrete endpoint with its original TCP/UDP port.
type ResolvedAddress struct {
	Address string
	Family  IPFamily
}

// ResolveAddressCandidates returns at most the useful concrete addresses for an
// endpoint. If both families exist, IPv6 is placed first and IPv4 immediately
// after it so a small stagger can recover quickly from a broken IPv6 path.
func ResolveAddressCandidates(ctx context.Context, address string, resolver IPResolver) ([]ResolvedAddress, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint %q: %w", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid endpoint port")
	}

	if ip := net.ParseIP(host); ip != nil {
		family := FamilyIPv6
		if ip.To4() != nil {
			family = FamilyIPv4
		}
		return []ResolvedAddress{{Address: net.JoinHostPort(ip.String(), portText), Family: family}}, nil
	}

	if resolver == nil {
		resolver = net.DefaultResolver
	}
	resolved, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}

	var firstV6, firstV4 *ResolvedAddress
	seen := make(map[string]struct{})
	for _, item := range resolved {
		ip := item.IP
		if ip == nil {
			continue
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		candidate := &ResolvedAddress{
			Address: net.JoinHostPort(key, portText),
			Family:  FamilyIPv6,
		}
		if ip.To4() != nil {
			candidate.Family = FamilyIPv4
			if firstV4 == nil {
				firstV4 = candidate
			}
		} else if firstV6 == nil {
			firstV6 = candidate
		}
	}

	out := make([]ResolvedAddress, 0, 2)
	if firstV6 != nil {
		out = append(out, *firstV6)
	}
	if firstV4 != nil {
		out = append(out, *firstV4)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no usable IP addresses for %s", host)
	}
	return out, nil
}

// TCPDialResult includes the family and connect latency used by diagnostics.
type TCPDialResult struct {
	Conn    net.Conn
	Family  IPFamily
	Latency time.Duration
}

// DialTCPHappyEyeballs races one IPv6 and one IPv4 candidate with a small
// stagger. No application bytes are sent until a winner is returned.
func DialTCPHappyEyeballs(
	ctx context.Context,
	address string,
	timeout time.Duration,
	stagger time.Duration,
	resolver IPResolver,
) (TCPDialResult, error) {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if stagger <= 0 {
		stagger = 100 * time.Millisecond
	}

	candidates, err := ResolveAddressCandidates(ctx, address, resolver)
	if err != nil {
		return TCPDialResult{}, err
	}

	attempts := make([]ConnAttempt, 0, len(candidates))
	families := make(map[string]IPFamily, len(candidates))
	for i, candidate := range candidates {
		candidate := candidate
		families[candidate.Address] = candidate.Family
		network := "tcp6"
		if candidate.Family == FamilyIPv4 {
			network = "tcp4"
		}
		dialer := &net.Dialer{Timeout: timeout}
		attempts = append(attempts, ConnAttempt{
			Name:  candidate.Address,
			Delay: time.Duration(i) * stagger,
			Dial: func(attemptCtx context.Context) (net.Conn, error) {
				return dialer.DialContext(attemptCtx, network, candidate.Address)
			},
		})
	}

	winner, _, err := RaceConnections(ctx, attempts)
	if err != nil {
		return TCPDialResult{}, err
	}
	return TCPDialResult{
		Conn:    winner.Conn,
		Family:  families[winner.Name],
		Latency: winner.Latency,
	}, nil
}
