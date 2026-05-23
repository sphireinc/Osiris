package osiris

import (
	"fmt"
	"strconv"
	"strings"
)

type semVersion struct {
	major int
	minor int
	patch int
	raw   string
}

// parseSemVersion handles the parse sem version step in the surrounding
// package and returns semVersion, error.
//
// It returns semVersion and an error. Callers should only trust the value when
// the error is nil.
func parseSemVersion(s string) (semVersion, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return semVersion{}, fmt.Errorf("empty version")
	}
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return semVersion{}, fmt.Errorf("invalid semver %q", s)
	}
	var out semVersion
	if len(parts) >= 1 {
		n, err := strconv.Atoi(parts[0])
		if err != nil {
			return semVersion{}, fmt.Errorf("invalid semver %q", s)
		}
		out.major = n
	}
	if len(parts) >= 2 {
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return semVersion{}, fmt.Errorf("invalid semver %q", s)
		}
		out.minor = n
	}
	if len(parts) == 3 {
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			return semVersion{}, fmt.Errorf("invalid semver %q", s)
		}
		out.patch = n
	}
	out.raw = normalizeSemVersion(out.major, out.minor, out.patch, len(parts))
	return out, nil
}

// normalizeSemVersion handles the normalize sem version step in the
// surrounding package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func normalizeSemVersion(major, minor, patch, parts int) string {
	switch parts {
	case 1:
		return fmt.Sprintf("%d", major)
	case 2:
		return fmt.Sprintf("%d.%d", major, minor)
	default:
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
}

// String operates on semVersion values and keeps the semversion-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (v semVersion) String() string {
	if v.raw != "" {
		return v.raw
	}
	return normalizeSemVersion(v.major, v.minor, v.patch, 3)
}

// MatchesSelector operates on semVersion values and keeps the
// semversion-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (v semVersion) MatchesSelector(selector string) bool {
	if selector == "" {
		return true
	}
	want, err := parseSemVersion(selector)
	if err != nil {
		return false
	}
	parts := strings.Count(selector, ".") + 1
	switch parts {
	case 1:
		return v.major == want.major
	case 2:
		return v.major == want.major && v.minor == want.minor
	default:
		return v.major == want.major && v.minor == want.minor && v.patch == want.patch
	}
}
