package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("AGENT_TESTBENCH_TEST_CLI") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
