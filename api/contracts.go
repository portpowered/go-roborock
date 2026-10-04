// Package api embeds the checked-in Roborock REST contracts for response validation.
package api

import "embed"

// RESTContracts contains the authentication and home inventory contracts.
//
//go:embed auth.openapi.yaml devices.openapi.yaml
var RESTContracts embed.FS
