// Package lucy provides integration interfaces for the Lucy Minecraft package manager.
//
// Lucy is StratumMC's non-intrusive dependency management layer. It handles:
//   - Manifest creation and management (lucy.yaml)
//   - Lock file reading and hash calculation for checkpoint metadata
//   - Artifact metadata extraction from uploaded JARs
//   - Direct package installation via Go API (no CLI subprocess needed)
//   - Server environment probing
//
// Lucy must remain strictly non-intrusive:
//   - Does NOT manage JVM processes
//   - Does NOT control server runtime
//   - Does NOT replace MCDR or Agent supervision
//
// For process lifecycle, see internal/agent/process.
// For MCDR integration, see internal/integration/mcdr.
package lucy
