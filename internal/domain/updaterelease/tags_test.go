package updaterelease

import "testing"

func TestLatestTagFromRemoteOutputIgnoresPrereleases(t *testing.T) {
	out := "aaa refs/tags/v1.9.0\nbbb refs/tags/v2.0.0-rc1\nccc refs/tags/v1.10.0\n"

	if got := LatestTagFromRemoteOutput(out); got != "v1.10.0" {
		t.Fatalf("latest stable release tag = %q", got)
	}
}

func TestTagsFromRemoteOutputDeduplicatesPeeledTags(t *testing.T) {
	out := "abc\trefs/tags/v1.0.0\nabc\trefs/tags/v1.0.0^{}\ndef\trefs/heads/main\nghi\trefs/tags/v1.1.0\n"

	tags := TagsFromRemoteOutput(out)
	if len(tags) != 2 || tags[0] != "v1.0.0" || tags[1] != "v1.1.0" {
		t.Fatalf("unexpected tags: %#v", tags)
	}
}

func TestLatestTagFromRemoteOutputPrefersHighestStableVersion(t *testing.T) {
	out := "a\trefs/tags/v1.9.0\nb\trefs/tags/v1.10.0-rc1\nc\trefs/tags/v1.10.0\nd\trefs/tags/not-version\n"

	if got := LatestTagFromRemoteOutput(out); got != "v1.10.0" {
		t.Fatalf("latest tag = %q, want v1.10.0", got)
	}
}

func TestCompareTagsOrdersPrereleasePartsNumerically(t *testing.T) {
	if CompareTags("v2.0.0-rc10", "v2.0.0-rc2") <= 0 {
		t.Fatal("rc10 should sort after rc2")
	}
	if CompareTags("v2.0.0", "v2.0.0-rc10") <= 0 {
		t.Fatal("stable release should sort after prerelease")
	}
}
