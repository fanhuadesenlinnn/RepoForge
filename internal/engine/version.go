package engine

import (
	"strings"
	"unicode"
)

// compareRPM compares two RPM (epoch:version-release) version strings.
// Returns -1, 0, +1.
func compareRPM(a, b string) int {
	ae := ""
	av := a
	ar := ""
	be := ""
	bv := b
	br := ""
	if i := strings.Index(a, ":"); i >= 0 {
		ae, av = a[:i], a[i+1:]
	}
	if i := strings.Index(b, ":"); i >= 0 {
		be, bv = b[:i], b[i+1:]
	}
	// release split
	if i := strings.LastIndex(av, "-"); i >= 0 {
		av, ar = av[:i], av[i+1:]
	}
	if i := strings.LastIndex(bv, "-"); i >= 0 {
		bv, br = bv[:i], bv[i+1:]
	}
	// epoch: higher wins
	if c := compareNumOrAlpha(ae, be); c != 0 {
		return c
	}
	if c := rpmvercmp(av, bv); c != 0 {
		return c
	}
	return rpmvercmp(ar, br)
}

func compareNumOrAlpha(a, b string) int {
	// both numeric or both empty
	an := a == "" || isDigits(a)
	bn := b == "" || isDigits(b)
	if an && bn {
		if a == "" {
			a = "0"
		}
		if b == "" {
			b = "0"
		}
		return compareNumerics(a, b)
	}
	// treat as strings (case-insensitive)
	la, lb := strings.ToLower(a), strings.ToLower(b)
	switch {
	case la < lb:
		return -1
	case la > lb:
		return 1
	}
	return 0
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func compareNumerics(a, b string) int {
	// strip leading zeros and compare length then lexicographic
	ta := strings.TrimLeft(a, "0")
	tb := strings.TrimLeft(b, "0")
	if ta == "" {
		ta = "0"
	}
	if tb == "" {
		tb = "0"
	}
	if len(ta) != len(tb) {
		if len(ta) < len(tb) {
			return -1
		}
		return 1
	}
	switch {
	case ta < tb:
		return -1
	case ta > tb:
		return 1
	}
	return 0
}

// rpmvercmp implements RPM's version string comparison by alternating
// numeric and alphanumeric runs. It always makes progress on each iteration.
func rpmvercmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// advance to next alphanumeric on each side
		ai, aj := i, j
		for ai < len(a) && !isAlnum(a[ai]) {
			ai++
		}
		for aj < len(b) && !isAlnum(b[aj]) {
			aj++
		}
		if ai >= len(a) && aj >= len(b) {
			return 0
		}
		aDigit := ai < len(a) && isDigit(a[ai])
		bDigit := aj < len(b) && isDigit(b[aj])
		if aDigit != bDigit {
			// numeric segment sorts after (newer than) alpha segment
			if aDigit {
				return 1
			}
			return -1
		}
		ni, nj := ai, aj
		for ni < len(a) && isAlnum(a[ni]) && isDigit(a[ni]) == aDigit {
			ni++
		}
		for nj < len(b) && isAlnum(b[nj]) && isDigit(b[nj]) == bDigit {
			nj++
		}
		if aDigit {
			if c := compareNumericRuns(a[ai:ni], b[aj:nj]); c != 0 {
				return c
			}
		} else if c := compareAlphaRuns(strings.ToLower(a[ai:ni]), strings.ToLower(b[aj:nj])); c != 0 {
			return c
		}
		i, j = ni, nj
	}
	return 0
}

func isAlnum(c byte) bool { return isDigit(c) || isAlpha(c) }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func compareNumericRuns(a, b string) int {
	return compareNumerics(a, b)
}

func compareAlphaRuns(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareDEB compares two Debian version strings (epoch:upstream-revision).
func compareDEB(a, b string) int {
	ae := ""
	au := a
	ar := ""
	be := ""
	bu := b
	br := ""
	if i := strings.Index(a, ":"); i >= 0 {
		ae, au = a[:i], a[i+1:]
	}
	if i := strings.Index(b, ":"); i >= 0 {
		be, bu = b[:i], b[i+1:]
	}
	if c := compareNumOrAlpha(ae, be); c != 0 {
		return c
	}
	if i := strings.Index(au, "-"); i >= 0 {
		au, ar = au[:i], au[i+1:]
	}
	if i := strings.Index(bu, "-"); i >= 0 {
		bu, br = bu[:i], bu[i+1:]
	}
	if c := debvercmp(au, bu); c != 0 {
		return c
	}
	return debvercmp(ar, br)
}

// debvercmp compares Debian upstream version strings with ~ and : handling.
func debvercmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// '~' sorts before everything
		if i < len(a) && a[i] == '~' {
			if j < len(b) && b[j] == '~' {
				i++
				j++
				continue
			}
			return -1
		}
		if j < len(b) && b[j] == '~' {
			return 1
		}
		if i < len(a) && isDigit(a[i]) {
			ni, nj := i, j
			for ni < len(a) && isDigit(a[ni]) {
				ni++
			}
			for nj < len(b) && isDigit(b[nj]) {
				nj++
			}
			if c := compareNumerics(a[i:ni], b[j:nj]); c != 0 {
				return c
			}
			i, j = ni, nj
			continue
		}
		// non-digit run
		if i < len(a) && j < len(b) {
			ca, cb := a[i], b[j]
			if ca != cb {
				if ca < cb {
					return -1
				}
				return 1
			}
			i++
			j++
			continue
		}
		if i < len(a) {
			return 1
		}
		return -1
	}
	return 0
}
