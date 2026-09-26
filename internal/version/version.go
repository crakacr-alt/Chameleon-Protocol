package version

import "strings"

// Value can be replaced at build time with:
// -ldflags "-X github.com/crakacr-alt/Chameleon-Protocol/internal/version.Value=vX.Y.Z"
var Value = "0.9.0"

func Normalize() string {
	v := strings.TrimSpace(Value)
	if v == "" {
		return "dev"
	}
	return v
}
