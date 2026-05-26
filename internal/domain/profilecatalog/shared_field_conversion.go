package profilecatalog

import (
	"reflect"

	domaincatalog "agent-testbench/internal/domain/catalog"
	"agent-testbench/internal/domain/profile"
)

const (
	sharedFieldSortOrder = "SortOrder"
	sharedFieldStatus    = "Status"
)

var apiCaseSharedFields = []string{
	"ID", "DisplayName", "Description", "NodeID", "CaseType", "Scenario", "Tags", "Priority", "Owner",
	"PayloadTemplateJSON", "RequestTemplateID", "PatchJSON", "RenderMode", "ExpectedJSON", "RequiredForAdmission",
	sharedFieldStatus, sharedFieldSortOrder, "CasePath", "SourceKind", "SourcePath", "ExecutorID", "BaseURL", "EvidenceDir", "TimeoutSeconds",
}

var serviceSharedFields = []string{
	"ID", "DisplayName", "Kind", "AttachedTemplateIDs", "GitURL", "GitBranch", "RepoEnv", "SourcePath",
	"ContainerName", "Image", "DockerService", "ServicePort", "ManagementPort", "MemoryMb", "CPUMilli",
	"StartupCommand", "HealthURL", "LogPath", sharedFieldStatus, sharedFieldSortOrder,
}

var templateConfigSharedFields = []string{
	"ID", "TemplateID", "NodeID", "WorkflowID", "ScopeType", "ScopeID", "Title", "Description", "ConfigJSON", sharedFieldStatus, sharedFieldSortOrder,
}

var interfaceNodeSharedFields = []string{
	"ID", "DisplayName", "ServiceID", "Operation", "Method", "Path", "TemplateID", "Version", sharedFieldStatus, "Tags",
	"Description", "TimeoutMs", sharedFieldSortOrder, "CreatedAt", "UpdatedAt",
}

func catalogServiceFromProfile(item profile.Service, runtimeEnv map[string]string) domaincatalog.Service {
	var out domaincatalog.Service
	copySharedFields(&out, item, serviceSharedFields)
	out.SourcePath = serviceSourcePath(runtimeEnv, item)
	return out
}

func profileServiceFromCatalog(item domaincatalog.Service) profile.Service {
	var out profile.Service
	copySharedFields(&out, item, serviceSharedFields)
	return out
}

func copySharedFields(dst any, src any, fieldNames []string) {
	dstValue := reflect.ValueOf(dst).Elem()
	srcValue := reflect.ValueOf(src)
	for _, fieldName := range fieldNames {
		dstValue.FieldByName(fieldName).Set(srcValue.FieldByName(fieldName))
	}
}
