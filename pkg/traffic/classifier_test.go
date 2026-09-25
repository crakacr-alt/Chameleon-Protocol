package traffic

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		hint Hint
		want Class
	}{
		{Hint{Destination: "example.com:443", Protocol: "tcp"}, ClassWeb},
		{Hint{Destination: "host:22", Protocol: "tcp"}, ClassInteractive},
		{Hint{Destination: "host:7777", Protocol: "udp"}, ClassRealtime},
		{Hint{Destination: "example.com:443", Protocol: "tcp", Purpose: "streaming"}, ClassStreaming},
		{Hint{Destination: "example.com:443", Protocol: "tcp", Purpose: "backup"}, ClassBulk},
	}

	for _, tc := range cases {
		if got := Classify(tc.hint); got != tc.want {
			t.Fatalf("%+v: want %s, got %s", tc.hint, tc.want, got)
		}
	}
}
