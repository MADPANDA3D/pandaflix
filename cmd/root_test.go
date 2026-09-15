package cmd

import "testing"

func TestPosterNameFromLabel(t *testing.T) {
	cases := map[string]string{
		"[series] The Blacklist (2013)": "The Blacklist (2013)",
		"[movie] The Goonies (1985)":    "The Goonies (1985)",
		"[movie] Title":                 "Title",
		"plain":                         "plain",
	}
	for label, want := range cases {
		if got := posterNameFromLabel(label); got != want {
			t.Errorf("posterNameFromLabel(%q) = %q, want %q", label, got, want)
		}
	}
}
