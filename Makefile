QUALITY_GATE_REPORT_DIR ?= build/reports/quality-gate
QUALITY_GATE_STRICT ?= false

.PHONY: quality quality-report quality-strict lint lint-full test

quality: quality-report

quality-report:
	QUALITY_GATE_REPORT_DIR="$(QUALITY_GATE_REPORT_DIR)" QUALITY_GATE_STRICT="$(QUALITY_GATE_STRICT)" scripts/quality-gate.sh

quality-strict:
	QUALITY_GATE_REPORT_DIR="$(QUALITY_GATE_REPORT_DIR)" QUALITY_GATE_STRICT=true scripts/quality-gate.sh

lint:
	tools/go-lint.sh

lint-full:
	tools/go-lint.sh --full

test:
	go test ./...
