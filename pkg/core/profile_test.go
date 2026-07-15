package core

import "testing"

func TestProfileDefaultsAreValid(t *testing.T) {
	t.Parallel()

	for _, profile := range []BehaviorProfile{ProfileWebRTC, ProfileHTTP3, ProfileGaming} {
		cfg := profile.Defaults()
		if err := cfg.Padding.Validate(); err != nil {
			t.Fatalf("profile %s: padding validation failed: %v", profile, err)
		}
		if err := cfg.Jitter.Validate(); err != nil {
			t.Fatalf("profile %s: jitter validation failed: %v", profile, err)
		}
	}
}
