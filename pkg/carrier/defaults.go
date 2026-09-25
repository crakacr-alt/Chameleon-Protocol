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
