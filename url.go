package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// repoNameFromURL derives the repository name from a clone URL. It understands
// the three shapes that show up in practice:
//
//	https://github.com/fordaj/AirFrame.git
//	git@github.com:fordaj/AirFrame.git
//	/srv/git/AirFrame.git
func repoNameFromURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}

	// Drop query strings and fragments, then trailing slashes.
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = strings.TrimRight(u, "/")
	if u == "" {
		return ""
	}

	// scp-like syntax ([user@]host:path) keeps the path in the same segment
	// as the host, so strip the host part. Scheme URLs ("https://") are
	// detected first and must not be mistaken for scp syntax.
	if i := strings.Index(u, ":"); i > 0 && !strings.HasPrefix(u[i:], "://") && !strings.Contains(u[:i], "/") {
		u = u[i+1:]
	}

	// Keep the last path segment.
	if i := strings.LastIndexAny(u, `/\`); i >= 0 {
		u = u[i+1:]
	}
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimSuffix(u, ".bundle")
	if u == "" {
		return ""
	}
	return sanitizeName(u)
}

// sanitizeName turns an arbitrary string into a name that is safe to use as a
// directory name on every supported platform.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "repo"
	}
	return out
}

// shortHash returns a short, stable hash of s. It is used to keep mirror
// cache entries for repositories that share a name apart from each other.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

// isRelativeURL reports whether a submodule URL has to be resolved against
// the URL of the superproject.
func isRelativeURL(u string) bool {
	return strings.HasPrefix(u, "./") || strings.HasPrefix(u, "../")
}

// resolveSubmoduleURL resolves a submodule URL against the URL of the
// superproject. Absolute URLs are returned unchanged. Relative URLs follow
// the rules of git: they are resolved against the superproject URL treated as
// a directory, so "../lib.git" next to "https://host/org/proj.git" becomes
// "https://host/org/lib.git".
func resolveSubmoduleURL(parentURL, subURL string) (string, error) {
	sub := strings.TrimSpace(subURL)
	if sub == "" {
		return "", fmt.Errorf("empty submodule url")
	}
	if !isRelativeURL(sub) {
		return sub, nil
	}
	if parentURL == "" {
		return "", fmt.Errorf("cannot resolve relative submodule url %q without a parent url", sub)
	}
	return resolveRelativeURL(parentURL, sub)
}

// resolveRelativeURL joins a relative URL onto base and normalizes the
// resulting path lexically (no filesystem access, the target may live on
// another host).
func resolveRelativeURL(base, rel string) (string, error) {
	head, tail := splitURLHead(base + "/" + rel)

	segments := strings.Split(tail, "/")
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		switch segment {
		case "", ".":
			continue
		case "..":
			if len(out) == 0 {
				return "", fmt.Errorf("cannot resolve %q against %q", rel, base)
			}
			out = out[:len(out)-1]
		default:
			out = append(out, segment)
		}
	}
	if len(out) == 0 {
		return head, nil
	}
	return head + strings.Join(out, "/"), nil
}

// splitURLHead splits a URL into the part that must never be resolved away
// (scheme and authority, or the host of an scp-like URL) and the path part.
// The head keeps its trailing separator so the path can be appended to it.
func splitURLHead(u string) (head, tail string) {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return u[:i+3+j+1], rest[j+1:]
		}
		return u, ""
	}
	// scp-like: [user@]host:path. Only the host is protected, the path is
	// resolved like every other path.
	if i := strings.Index(u, ":"); i > 0 && !strings.Contains(u[:i], "/") {
		return u[:i+1], u[i+1:]
	}
	// Local path.
	return "/", strings.TrimPrefix(u, "/")
}

// sameURL compares two repository URLs after normalizing the parts that do
// not change which repository is addressed.
func sameURL(a, b string) bool {
	normalize := func(u string) string {
		u = strings.TrimSpace(u)
		u = strings.TrimRight(u, "/")
		if len(u) > 4 && strings.EqualFold(u[:4], "file:") {
			u = strings.TrimPrefix(u, "file://")
		}
		return u
	}
	return normalize(a) == normalize(b)
}
