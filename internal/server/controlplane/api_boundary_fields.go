package controlplane

// Map-backed API payloads use these field names at the control-plane boundary.
// Keeping the vocabulary here makes trusted execution fields and response
// metadata consistent without coupling domain structs to transport details.
const (
	apiFieldCode           = "code"
	apiFieldDescription    = "description"
	apiFieldEvidence       = "evidence"
	apiFieldEvidenceDir    = "evidenceDir"
	apiFieldTimeoutSeconds = "timeoutSeconds"
	apiFieldTitle          = "title"
)

// API case runners and Store indexing share one Evidence filename contract.
const (
	apiCaseEvidenceFileCase       = "case.json"
	apiCaseEvidenceFileRequest    = "request.json"
	apiCaseEvidenceFileResponse   = "response.json"
	apiCaseEvidenceFileAssertions = "assertions.json"
	apiCaseEvidenceFileError      = "error.json"
	apiCaseEvidenceFileSummary    = "summary.json"
)

func apiCaseEvidenceFiles() []string {
	return []string{
		apiCaseEvidenceFileCase,
		apiCaseEvidenceFileRequest,
		apiCaseEvidenceFileResponse,
		apiCaseEvidenceFileAssertions,
		apiCaseEvidenceFileError,
		apiCaseEvidenceFileSummary,
	}
}
