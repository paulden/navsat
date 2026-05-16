package aws

import "testing"

func TestArchFromInstanceType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Graviton (arm64)
		{"t4g.nano", "arm64"},
		{"t4g.micro", "arm64"},
		{"m6g.medium", "arm64"},
		{"c6g.large", "arm64"},
		{"r6g.2xlarge", "arm64"},
		{"x2g.medium", "arm64"},
		{"a1.medium", "arm64"},
		// x86_64
		{"t3.nano", "x86_64"},
		{"t3a.micro", "x86_64"}, // ends in 'a', not 'g'
		{"m5.xlarge", "x86_64"},
		{"c5.large", "x86_64"},
		{"r5.2xlarge", "x86_64"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := archFromInstanceType(tc.in)
			if got != tc.want {
				t.Errorf("archFromInstanceType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
