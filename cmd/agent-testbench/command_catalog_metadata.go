package main

const (
	cliCommandStatus   = "status"
	cliCommandCommands = onboardSmokeCommands
	cliCommandDoctor   = "doctor"

	commandCatalogEnvironmentRestore           = "environment restore"
	commandCatalogEnvironmentStatus            = "environment status"
	commandCatalogEnvironmentStop              = "environment stop"
	commandCatalogEnvironmentRestart           = "environment service restart"
	commandCatalogEnvironmentConfigure         = "environment configure"
	commandCatalogEnvironmentRepoSet           = "environment repo set"
	commandCatalogEnvironmentStartupFilePut    = "environment startup-file put"
	commandCatalogEnvironmentComponentsInspect = "environment components inspect"
	commandCatalogEnvironmentComponentsReplace = "environment components replace"
	commandCatalogTaskPlan                     = "task plan"
	commandCatalogWorkflowGate                 = "workflow gate"
	commandCatalogCaseInspect                  = "case inspect"
	commandCatalogCaseDiagnose                 = "case diagnose"
	commandCatalogCaseGate                     = "case gate"
	commandCatalogCaseRun                      = "case run"
	commandCatalogCaseSuiteReport              = "case suite report"
	commandCatalogExecutorPlan                 = "executor plan"
	commandCatalogEvidenceInspect              = "evidence inspect"
	commandCatalogEvidenceList                 = "evidence list"
	commandCatalogEvidenceTasks                = "evidence tasks"

	commandCatalogMapList              = "map list"
	commandCatalogMapWorkflows         = "map workflows"
	commandCatalogMapCoverage          = "map coverage"
	commandCatalogMapPlans             = "map plans"
	commandCatalogMapVersions          = "map versions"
	commandCatalogMapImportWorkflows   = "map import-workflows"
	commandCatalogMapDoctor            = "map doctor"
	commandCatalogMapDiff              = "map diff"
	commandCatalogMapValidationList    = "map validation list"
	commandCatalogMapValidationAttach  = "map validation attach"
	commandCatalogMapValidationPromote = "map validation promote"
	commandCatalogMapUpdate            = "map update"
	commandCatalogMapSnapshot          = "map snapshot"
	commandCatalogMapPublish           = "map publish"
	commandCatalogMapInspect           = "map inspect"
	commandCatalogMapExplain           = "map explain"
	commandCatalogMapPlanInspect       = "map plan inspect"
	commandCatalogMapRun               = "map run"
	commandCatalogMapGate              = "map gate"
	commandCatalogMapAtlas             = "map atlas"

	commandCatalogLifecycleInspect  = "inspect"
	commandCatalogLifecycleMaintain = "maintain"
	commandCatalogLifecyclePlan     = "plan"
	commandCatalogLifecycleExecute  = "execute"
	commandCatalogLifecycleReview   = "review"

	workflowToMapImportReplacement = "agent-testbench map import-workflows --workflow WORKFLOW_ID --map MAP_ID"

	commandCatalogSurfaceDefault       = "default"
	commandCatalogSurfaceExtended      = "extended"
	commandCatalogSurfaceInternal      = "internal"
	commandCatalogSurfaceCompatibility = "compatibility"
	commandCatalogSurfaceDeprecated    = "deprecated"
)

func commandCatalogMapLifecycle(command string) string {
	return commandCatalogMapLifecycles()[command]
}

func commandCatalogMapLifecycles() map[string]string {
	return map[string]string{
		commandCatalogMapList:              commandCatalogLifecycleInspect,
		commandCatalogMapWorkflows:         commandCatalogLifecycleInspect,
		commandCatalogMapCoverage:          commandCatalogLifecycleInspect,
		commandCatalogMapPlans:             commandCatalogLifecycleInspect,
		commandCatalogMapVersions:          commandCatalogLifecycleInspect,
		commandCatalogMapImportWorkflows:   commandCatalogLifecycleMaintain,
		commandCatalogMapDoctor:            commandCatalogLifecycleMaintain,
		commandCatalogMapDiff:              commandCatalogLifecycleMaintain,
		commandCatalogMapValidationList:    commandCatalogLifecycleMaintain,
		commandCatalogMapValidationAttach:  commandCatalogLifecycleMaintain,
		commandCatalogMapValidationPromote: commandCatalogLifecycleMaintain,
		commandCatalogMapUpdate:            commandCatalogLifecycleMaintain,
		commandCatalogMapSnapshot:          commandCatalogLifecycleMaintain,
		commandCatalogMapPublish:           commandCatalogLifecycleMaintain,
		commandCatalogMapInspect:           commandCatalogLifecycleInspect,
		commandCatalogMapExplain:           commandCatalogLifecyclePlan,
		commandCatalogMapPlanInspect:       commandCatalogLifecyclePlan,
		commandCatalogMapRun:               commandCatalogLifecycleExecute,
		commandCatalogMapGate:              commandCatalogLifecycleExecute,
		commandCatalogMapAtlas:             commandCatalogLifecycleReview,
	}
}

func commandCatalogTaskRank(command string) int {
	return commandCatalogTaskRanks()[command]
}

func commandCatalogTaskRanks() map[string]int {
	return map[string]int{
		commandCatalogMapDoctor:            10,
		commandCatalogMapCoverage:          20,
		commandCatalogMapDiff:              30,
		commandCatalogMapValidationList:    40,
		commandCatalogMapValidationAttach:  50,
		commandCatalogMapValidationPromote: 55,
		commandCatalogMapUpdate:            60,
		commandCatalogMapSnapshot:          70,
		commandCatalogMapPublish:           80,
		commandCatalogMapVersions:          90,
		commandCatalogMapImportWorkflows:   100,
		commandCatalogMapInspect:           105,
		commandCatalogMapList:              110,
		commandCatalogMapWorkflows:         120,
		commandCatalogMapExplain:           210,
		commandCatalogMapPlanInspect:       220,
		commandCatalogMapRun:               230,
		commandCatalogMapGate:              240,
		commandCatalogMapPlans:             260,
		commandCatalogMapAtlas:             310,
	}
}
