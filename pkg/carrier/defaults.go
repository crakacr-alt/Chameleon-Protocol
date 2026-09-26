package carrier

// Defaults builds the normal route list.
// Empty endpoints are omitted so the caller can add only the services
// that really exist on this machine.
func Defaults(chameleonUDP, chameleonTCP, relay string) []Candidate {
	out := []Candidate{
		{
			Name:        "direct",
			Kind:        KindDirect,
			Cost:        0,
			SupportsTCP: true,
			SupportsUDP: true,
		},
	}

	if chameleonUDP != "" {
		out = append(out, Candidate{
			Name:        "chameleon-udp",
			Kind:        KindChameleonUDP,
			Endpoint:    chameleonUDP,
			Cost:        0.35,
			SupportsTCP: true,
			SupportsUDP: true,
		})
	}
	if chameleonTCP != "" {
		out = append(out, Candidate{
			Name:        "chameleon-tcp",
			Kind:        KindChameleonTCP,
			Endpoint:    chameleonTCP,
			Cost:        0.55,
			SupportsTCP: true,
			SupportsUDP: false,
		})
	}
	if relay != "" {
		out = append(out, Candidate{
			Name:             "relay",
			Kind:             KindRelay,
			Endpoint:         relay,
			Cost:             0.9,
			SupportsTCP:      true,
			SupportsUDP:      true,
			RequiresExternal: true,
		})
	}

	return out
}

// WithTLS appends an optional TLS-fronted Chameleon carrier.
// It is separate from Defaults to preserve existing callers and configs.
func WithTLS(candidates []Candidate, endpoint string) []Candidate {
	if endpoint == "" {
		return candidates
	}
	return append(candidates, Candidate{
		Name:        "chameleon-tls",
		Kind:        KindChameleonTLS,
		Endpoint:    endpoint,
		Cost:        0.45,
		SupportsTCP: true,
		SupportsUDP: false,
	})
}

// WithQUIC appends the real UDP/QUIC Chameleon carrier.
// QUIC can carry reliable TCP-style proxy streams today and is marked UDP
// capable for the datagram executor introduced by the 0.9 line.
func WithQUIC(candidates []Candidate, endpoint string) []Candidate {
	if endpoint == "" {
		return candidates
	}
	return append(candidates, Candidate{
		Name:        "chameleon-quic",
		Kind:        KindChameleonQUIC,
		Endpoint:    endpoint,
		Cost:        0.30,
		SupportsTCP: true,
		SupportsUDP: true,
	})
}
