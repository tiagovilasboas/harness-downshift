// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package models

import (
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// CatalogReader is the interface the models subcommands (list, check, pull)
// require from the catalog. It is intentionally separate from core.Resolver:
// Resolver is for routing decisions; CatalogReader is for human-facing catalog
// inspection and maintenance commands.
//
// *catalog.Catalog satisfies both interfaces. Any value that implements both
// can be passed to runModels in main.
type CatalogReader interface {
	// Entries returns all raw catalog entries, including unknown-tier ones.
	Entries() []catalog.Entry

	// LookupByID returns the model with the given ID in the given harness.
	LookupByID(harness, modelID string) (core.Model, bool)
}
