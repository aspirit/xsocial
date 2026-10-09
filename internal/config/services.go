// Package config defines service identifiers and configuration access methods
//
// Design Philosophy - Service Name Constants + Dynamic Configuration:
//
// 1. SERVICE NAME CONSTANTS (Go code): Define service names for type-safe code references
//   - Used in: switch statements, validation, code logic
//   - Example: config.ServiceAPI, config.ServiceWorker
//   - Benefits: Compile-time safety, IDE support, refactoring
//
// 2. SERVICE IDs (YAML config): Define runtime identifiers from configuration
//   - Used in: logging, tracing, monitoring, service discovery
//   - Example: "xs-api", "xs-api-dev", "xs-api-prod"
//   - Benefits: Environment flexibility, no hardcoded values
package config

import (
	"github.com/aspirit/xsocial/internal/storage/mongo"
)

// Service identifier constants
// These constants should match the keys in config.Services struct.
const (
	ServiceAPI    = "api"
	ServiceWorker = "worker"
)

// ServiceDatabaseRequirements maps services to the logical databases they open
// at boot (dep.BaseDeps). A service never touches a database it did not declare.
var ServiceDatabaseRequirements = map[string][]string{
	ServiceAPI:    {mongo.DBCore},
	ServiceWorker: {mongo.DBCore},
}

// GetServiceDatabases returns the list of databases required by a service.
func GetServiceDatabases(serviceName string) []string {
	databases, exists := ServiceDatabaseRequirements[serviceName]
	if !exists {
		return []string{mongo.DBCore} // Default to core database
	}

	return databases
}
