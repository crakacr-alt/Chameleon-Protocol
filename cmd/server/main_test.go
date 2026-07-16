package main

import "testing"

func TestNormalizeListenAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ":9000"},
		{name: "bind all", in: ":9000", want: ":9000"},
		{name: "host port", in: "127.0.0.1:9000", want: "127.0.0.1:9000"},
		{name: "host only", in: "127", want: "127:9000"},
		{name: "localhost only", in: "localhost", want: "localhost:9000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeListenAddress(tt.in); got != tt.want {
				t.Fatalf("normalizeListenAddress(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
