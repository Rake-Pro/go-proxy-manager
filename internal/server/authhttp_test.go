package server

import "testing"

func TestSanitizeReturnTo(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/dashboard", "/dashboard"},
		{"/a/b?x=1", "/a/b?x=1"},
		{"", "/"},
		{"relative", "/"},
		{"//evil.com", "/"},
		{`/\evil.com`, "/"},
		{`\/evil.com`, "/"},
		{`\evil.com`, "/"},
		{"https://evil.com", "/"},
		{"/ok\r\nSet-Cookie: x", "/"},
		{"/ok\x00", "/"},
	}
	for _, c := range cases {
		if got := sanitizeReturnTo(c.in); got != c.want {
			t.Errorf("sanitizeReturnTo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
