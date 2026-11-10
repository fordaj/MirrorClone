package main

import "testing"

func TestRepoNameFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/fordaj/AirFrame.git", "AirFrame"},
		{"https://github.com/fordaj/AirFrame", "AirFrame"},
		{"https://github.com/fordaj/AirFrame.git/", "AirFrame"},
		{"git@github.com:fordaj/AirFrame.git", "AirFrame"},
		{"ssh://git@github.com/fordaj/AirFrame.git", "AirFrame"},
		{"file:///srv/git/AirFrame.git", "AirFrame"},
		{"/srv/git/AirFrame.git", "AirFrame"},
		{"../relative/AirFrame.git", "AirFrame"},
		{"https://github.com/fordaj/air-frame.git", "air-frame"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := repoNameFromURL(tc.url); got != tc.want {
			t.Errorf("repoNameFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestResolveSubmoduleURL(t *testing.T) {
	cases := []struct {
		parent string
		sub    string
		want   string
	}{
		{
			parent: "https://github.com/fordaj/AirFrame.git",
			sub:    "../Other.git",
			want:   "https://github.com/fordaj/Other.git",
		},
		{
			parent: "https://github.com/fordaj/AirFrame.git",
			sub:    "./nested/Other.git",
			want:   "https://github.com/fordaj/AirFrame.git/nested/Other.git",
		},
		{
			parent: "git@github.com:fordaj/AirFrame.git",
			sub:    "../Other.git",
			want:   "git@github.com:fordaj/Other.git",
		},
		{
			parent: "/srv/git/AirFrame.git",
			sub:    "../Other.git",
			want:   "/srv/git/Other.git",
		},
		{
			parent: "https://github.com/fordaj/AirFrame.git",
			sub:    "https://example.com/Other.git",
			want:   "https://example.com/Other.git",
		},
		{
			parent: "git@github.com:fordaj/AirFrame.git",
			sub:    "git@example.com:group/Other.git",
			want:   "git@example.com:group/Other.git",
		},
	}

	for _, tc := range cases {
		got, err := resolveSubmoduleURL(tc.parent, tc.sub)
		if err != nil {
			t.Errorf("resolveSubmoduleURL(%q, %q) returned error: %v", tc.parent, tc.sub, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveSubmoduleURL(%q, %q) = %q, want %q", tc.parent, tc.sub, got, tc.want)
		}
	}
}

func TestResolveSubmoduleURLErrors(t *testing.T) {
	if _, err := resolveSubmoduleURL("", "../Other.git"); err == nil {
		t.Error("expected an error for a relative url without a parent")
	}
	if _, err := resolveSubmoduleURL("https://host/org/repo.git", "../../../Other.git"); err == nil {
		t.Error("expected an error for a url that escapes the root")
	}
}

func TestSameURL(t *testing.T) {
	cases := []struct {
		a    string
		b    string
		same bool
	}{
		{"https://github.com/fordaj/AirFrame.git", "https://github.com/fordaj/AirFrame.git", true},
		{"https://github.com/fordaj/AirFrame.git", "https://github.com/fordaj/AirFrame.git/", true},
		{"https://github.com/fordaj/AirFrame.git", "https://github.com/fordaj/Other.git", false},
		{"git@github.com:fordaj/AirFrame.git", "git@github.com:fordaj/AirFrame.git", true},
	}

	for _, tc := range cases {
		if got := sameURL(tc.a, tc.b); got != tc.same {
			t.Errorf("sameURL(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.same)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"AirFrame":         "AirFrame",
		"air frame":        "air-frame",
		"../escape":        "escape",
		"weird:name*here":  "weird-name-here",
		"":                 "repo",
		"...":              "repo",
		"sub/dir":          "sub-dir",
		"trailing.":        "trailing",
		"UPPER_and-dots.1": "UPPER_and-dots.1",
	}

	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
