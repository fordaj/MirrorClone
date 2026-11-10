package main

import "testing"

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Options
	}{
		{
			name: "repository only",
			args: []string{"git@github.com:fordaj/AirFrame.git"},
			want: Options{Repository: "git@github.com:fordaj/AirFrame.git", LogLevel: LevelInfo},
		},
		{
			name: "repository and directory",
			args: []string{"repo.git", "/tmp/dest"},
			want: Options{Repository: "repo.git", Directory: "/tmp/dest", LogLevel: LevelInfo},
		},
		{
			name: "flags before arguments",
			args: []string{"--branch", "main", "repo.git"},
			want: Options{Repository: "repo.git", Branch: "main", LogLevel: LevelInfo},
		},
		{
			name: "flags after arguments",
			args: []string{"repo.git", "--branch", "main"},
			want: Options{Repository: "repo.git", Branch: "main", LogLevel: LevelInfo},
		},
		{
			name: "long flag with equals",
			args: []string{"--branch=main", "repo.git"},
			want: Options{Repository: "repo.git", Branch: "main", LogLevel: LevelInfo},
		},
		{
			name: "short flag",
			args: []string{"-b", "main", "repo.git"},
			want: Options{Repository: "repo.git", Branch: "main", LogLevel: LevelInfo},
		},
		{
			name: "short flag with attached value",
			args: []string{"-bmain", "repo.git"},
			want: Options{Repository: "repo.git", Branch: "main", LogLevel: LevelInfo},
		},
		{
			name: "mirror cache dir",
			args: []string{"--mirror-cache-dir", "/tmp/cache", "repo.git"},
			want: Options{Repository: "repo.git", MirrorCacheDir: "/tmp/cache", LogLevel: LevelInfo},
		},
		{
			name: "jobs",
			args: []string{"--jobs", "4", "repo.git"},
			want: Options{Repository: "repo.git", Jobs: 4, LogLevel: LevelInfo},
		},
		{
			name: "quiet",
			args: []string{"-q", "repo.git"},
			want: Options{Repository: "repo.git", LogLevel: LevelError},
		},
		{
			name: "log level",
			args: []string{"--log-level", "debug", "repo.git"},
			want: Options{Repository: "repo.git", LogLevel: LevelDebug},
		},
		{
			name: "double dash ends the flags",
			args: []string{"--", "--weird-repo"},
			want: Options{Repository: "--weird-repo", LogLevel: LevelInfo},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if err != nil {
				t.Fatalf("parseArgs(%v) returned error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("parseArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"--unknown", "repo.git"},
		{"-x", "repo.git"},
		{"--branch"},
		{"--jobs", "zero", "repo.git"},
		{"--jobs", "0", "repo.git"},
		{"repo.git", "dest", "extra"},
	}

	for _, args := range cases {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) did not return an error", args)
		}
	}
}

func TestParseArgsHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		opts, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%v) returned error: %v", args, err)
		}
		if !opts.ShowHelp {
			t.Errorf("parseArgs(%v) did not set ShowHelp", args)
		}
	}

	for _, args := range [][]string{{"--version"}, {"-v"}} {
		opts, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%v) returned error: %v", args, err)
		}
		if !opts.ShowVersion {
			t.Errorf("parseArgs(%v) did not set ShowVersion", args)
		}
	}
}
