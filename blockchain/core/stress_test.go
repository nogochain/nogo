package core

import (
	"testing"
)

// All tests in this file are skipped because they require:
// 1. Proper genesis configuration
// 2. Storage backend (creates import cycle)
// 3. Complete blockchain setup
// TODO: Create proper stress tests with mock interfaces

func TestHighThroughputMining(t *testing.T) {
	t.Skip("requires complete blockchain setup with genesis configuration")
}

func TestStateRootPerformance(t *testing.T) {
	t.Skip("requires storage package - import cycle in test")
}

func TestRaceDetection(t *testing.T) {
	t.Skip("requires complete blockchain setup with genesis configuration")
}

func TestLargeStateRoot(t *testing.T) {
	t.Skip("requires storage package - import cycle in test")
}
