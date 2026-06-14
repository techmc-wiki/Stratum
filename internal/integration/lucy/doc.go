// Package lucy defines the Stratum-owned boundary for future Lucy adapters.
//
// The package contains stable request and response DTOs only. Implementations
// may later use a CLI, a Go package, or another transport, but callers must not
// depend on Lucy's internal provider or manifest types.
package lucy
