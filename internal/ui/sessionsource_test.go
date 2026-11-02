package ui

import "testing"

func TestCurrentRemoteSessionID(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"unset", "", ""},
		{"id", "3a2a58e442b070f8", "3a2a58e442b070f8"},
		{"path", "/run/user/1000/sshex/sessions/3a2a58e442b070f8.json", "3a2a58e442b070f8"},
		{"spaces", "  abc  ", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SSHEX_SESSION", tc.value)
			if got := currentRemoteSessionID(); got != tc.want {
				t.Fatalf("currentRemoteSessionID() = %q, want %q", got, tc.want)
			}
		})
	}
}
