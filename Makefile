QUALITY_GATE_REPORT_DIR ?= build/reports/quality-gate

.PHONY: quality quality-report quality-strict lint test

quality: quality-report

quality-report:
	QUALITY_GATE_REPORT_DIR="$(QUALITY_GATE_REPORT_DIR)" QUALITY_GATE_STRICT=false scripts/quality-gate.sh

quality-strict:
	QUALITY_GATE_REPORT_DIR="$(QUALITY_GATE_REPORT_DIR)" QUALITY_GATE_STRICT=true scripts/quality-gate.sh

lint:
	golangci-lint run

test:
	go test ./...
