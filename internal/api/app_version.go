package api

import "strings"

func parseVersionSegments(version string) []int {
	core := strings.TrimSpace(strings.SplitN(version, "+", 2)[0])
	if core == "" {
		return nil
	}
	parts := strings.Split(core, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n := 0
		for _, ch := range strings.TrimSpace(p) {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		out = append(out, n)
	}
	return out
}

// compareSemanticVersions 比较 a 相对 b：小为 -1，大为 1，否则 0（与客户端一致）。
func compareSemanticVersions(a, b string) int {
	pa := parseVersionSegments(a)
	pb := parseVersionSegments(b)
	lenA := len(pa)
	lenB := len(pb)
	maxLen := lenA
	if lenB > maxLen {
		maxLen = lenB
	}
	for i := 0; i < maxLen; i++ {
		va := 0
		if i < lenA {
			va = pa[i]
		}
		vb := 0
		if i < lenB {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}
