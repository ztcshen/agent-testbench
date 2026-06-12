// Package updaterelease selects release tags from Git remote tag listings.
package updaterelease

import (
	"sort"
	"strconv"
	"strings"
)

func LatestTagFromRemoteOutput(out string) string {
	tags := TagsFromRemoteOutput(out)
	tags = stableReleaseTags(tags)
	if len(tags) == 0 {
		return ""
	}
	sort.SliceStable(tags, func(i int, j int) bool {
		return CompareTags(tags[i], tags[j]) > 0
	})
	return tags[0]
}

func TagsFromRemoteOutput(out string) []string {
	seen := map[string]bool{}
	tags := []string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "refs/tags/") {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		tag = strings.TrimSuffix(tag, "^{}")
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func stableReleaseTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		version := parseTag(tag)
		if version.valid && version.prerelease == "" {
			out = append(out, tag)
		}
	}
	return out
}

func CompareTags(a string, b string) int {
	av := parseTag(a)
	bv := parseTag(b)
	if av.valid != bv.valid {
		if av.valid {
			return 1
		}
		return -1
	}
	if av.valid && bv.valid {
		maxParts := len(av.parts)
		if len(bv.parts) > maxParts {
			maxParts = len(bv.parts)
		}
		for i := 0; i < maxParts; i++ {
			ai := tagPart(av.parts, i)
			bi := tagPart(bv.parts, i)
			if ai > bi {
				return 1
			}
			if ai < bi {
				return -1
			}
		}
		if av.prerelease == "" && bv.prerelease != "" {
			return 1
		}
		if av.prerelease != "" && bv.prerelease == "" {
			return -1
		}
		if av.prerelease != "" && bv.prerelease != "" {
			if result := comparePrerelease(av.prerelease, bv.prerelease); result != 0 {
				return result
			}
		}
	}
	return strings.Compare(a, b)
}

func comparePrerelease(a string, b string) int {
	if a == b {
		return 0
	}
	aParts := splitPrerelease(a)
	bParts := splitPrerelease(b)
	maxParts := len(aParts)
	if len(bParts) > maxParts {
		maxParts = len(bParts)
	}
	for i := 0; i < maxParts; i++ {
		if i >= len(aParts) {
			return -1
		}
		if i >= len(bParts) {
			return 1
		}
		if result := comparePrereleasePart(aParts[i], bParts[i]); result != 0 {
			return result
		}
	}
	return strings.Compare(a, b)
}

func splitPrerelease(value string) []string {
	rawParts := strings.FieldsFunc(value, func(item rune) bool {
		return item == '.' || item == '-' || item == '_'
	})
	parts := make([]string, 0, len(rawParts))
	for _, rawPart := range rawParts {
		parts = append(parts, splitPrereleasePart(rawPart)...)
	}
	return parts
}

func splitPrereleasePart(value string) []string {
	if value == "" {
		return nil
	}
	parts := []string{}
	start := 0
	lastNumeric := value[0] >= '0' && value[0] <= '9'
	for index := 1; index < len(value); index++ {
		currentNumeric := value[index] >= '0' && value[index] <= '9'
		if currentNumeric == lastNumeric {
			continue
		}
		parts = append(parts, value[start:index])
		start = index
		lastNumeric = currentNumeric
	}
	parts = append(parts, value[start:])
	return parts
}

func comparePrereleasePart(a string, b string) int {
	aNumber, aNumeric := parsePrereleaseNumber(a)
	bNumber, bNumeric := parsePrereleaseNumber(b)
	if aNumeric && bNumeric {
		if aNumber > bNumber {
			return 1
		}
		if aNumber < bNumber {
			return -1
		}
		return 0
	}
	if aNumeric != bNumeric {
		if aNumeric {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func parsePrereleaseNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, item := range value {
		if item < '0' || item > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

type tagVersion struct {
	valid      bool
	parts      []int
	prerelease string
}

func parseTag(tag string) tagVersion {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tag), "v"), "V")
	if trimmed == "" {
		return tagVersion{}
	}
	versionPart := ""
	prerelease := ""
	releasePart := trimmed
	if index := strings.Index(releasePart, "+"); index >= 0 {
		releasePart = releasePart[:index]
	}
	if index := strings.IndexAny(releasePart, "-_"); index >= 0 {
		versionPart = releasePart[:index]
		prerelease = releasePart[index+1:]
	} else {
		versionPart = releasePart
	}
	rawParts := strings.Split(versionPart, ".")
	parts := make([]int, 0, len(rawParts))
	for _, raw := range rawParts {
		if !tagPartIsNumeric(raw) {
			return tagVersion{}
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return tagVersion{}
		}
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return tagVersion{}
	}
	return tagVersion{valid: true, parts: parts, prerelease: prerelease}
}

func tagPartIsNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, item := range value {
		if item < '0' || item > '9' {
			return false
		}
	}
	return true
}

func tagPart(parts []int, index int) int {
	if index >= len(parts) {
		return 0
	}
	return parts[index]
}
