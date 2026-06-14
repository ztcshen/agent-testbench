package store

import "testing"

func TestEnvironmentFilesFromComposeJSONNormalizesGeneratedFileKeysForContentLookup(t *testing.T) {
	files := EnvironmentFilesFromComposeJSON(map[string]any{
		"composeFiles": []any{"compose.yml"},
		"generatedFiles": map[string]any{
			"./compose.yml": "services:\n  app:\n    image: alpine:3.20\n",
		},
	}, "test")

	if len(files) != 1 {
		t.Fatalf("files = %#v, want one compose file", files)
	}
	if files[0].Path != "compose.yml" || files[0].Kind != EnvironmentFileKindComposeFile {
		t.Fatalf("file = %#v, want normalized compose.yml compose file", files[0])
	}
	if files[0].ContentInline == "" || !EnvironmentFileHasInlineContent(files[0]) {
		t.Fatalf("compose file should preserve inline content after path normalization: %#v", files[0])
	}
}
