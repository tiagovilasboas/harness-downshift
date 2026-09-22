// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package domain

import "context"

// Router is the interface for task routing implementations.
// Both LegacyRouter (wrapping core.Route) and CapabilityRouter implement this.
type Router interface {
	// Route classifies the task and returns the routing decision.
	// Implementations must be fail-open: errors should result in conservative
	// routing (higher tier), never blocking the subagent spawn.
	Route(ctx context.Context, input RoutingInput) (RoutingDecision, error)
}
