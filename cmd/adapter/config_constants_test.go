package main

import "testing"

func TestNormalizePassRefRoot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "pass://Personal/Proton Git LFS", want: "pass://Personal/Proton Git LFS"},
		{in: "pass://Personal/Proton Git LFS/", want: "pass://Personal/Proton Git LFS"},
		{in: "  pass://Personal/Proton Git LFS/  ", want: "pass://Personal/Proton Git LFS"},
	}

	for _, tc := range cases {
		got := normalizePassRefRoot(tc.in)
		if got != tc.want {
			t.Fatalf("normalizePassRefRoot(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDefaultPassRefs(t *testing.T) {
	root := "pass://Personal/Proton Git LFS"
	if got := defaultPassUsernameRef(root); got != "pass://Personal/Proton Git LFS/username" {
		t.Fatalf("unexpected username ref %q", got)
	}
	if got := defaultPassPasswordRef(root); got != "pass://Personal/Proton Git LFS/password" {
		t.Fatalf("unexpected password ref %q", got)
	}
}
