// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package extractor

import (
	"testing"
)

func TestExtract_MechanicalTask(t *testing.T) {
	prompt := "rename the variable userId to userID and fix the typo in the comment"
	fv := Extract(prompt)

	if fv.Mechanical < 0.3 {
		t.Errorf("mechanical task should have high Mechanical signal, got %f", fv.Mechanical)
	}
}

func TestExtract_SecurityTask(t *testing.T) {
	prompt := "implement JWT authentication with password hashing and role-based access control"
	fv := Extract(prompt)

	if fv.Security < 0.5 {
		t.Errorf("security task should have high Security signal, got %f", fv.Security)
	}
}

func TestExtract_ConcurrencyTask(t *testing.T) {
	prompt := "fix the race condition in the goroutine that updates the shared map without mutex"
	fv := Extract(prompt)

	if fv.Concurrency < 0.5 {
		t.Errorf("concurrency task should have high Concurrency signal, got %f", fv.Concurrency)
	}
	if fv.Debugging < 0.2 {
		t.Errorf("fix task should have Debugging signal, got %f", fv.Debugging)
	}
}

func TestExtract_MigrationTask(t *testing.T) {
	prompt := "migrate the database schema to add a new column and transform existing data"
	fv := Extract(prompt)

	if fv.Migration < 0.5 {
		t.Errorf("migration task should have high Migration signal, got %f", fv.Migration)
	}
}

func TestExtract_ArchitectureTask(t *testing.T) {
	prompt := "design a scalable architecture with proper layering and dependency injection patterns"
	fv := Extract(prompt)

	if fv.Architecture < 0.5 {
		t.Errorf("architecture task should have high Architecture signal, got %f", fv.Architecture)
	}
}

func TestExtract_AmbiguousTask(t *testing.T) {
	prompt := "fix it"
	fv := Extract(prompt)

	if fv.Ambiguity < 0.3 {
		t.Errorf("short vague prompt should have high Ambiguity signal, got %f", fv.Ambiguity)
	}
}

func TestExtract_CrossModuleTask(t *testing.T) {
	prompt := "refactor the codebase to use the new pattern across all modules and packages"
	fv := Extract(prompt)

	if fv.CrossModule < 0.4 {
		t.Errorf("cross-module task should have high CrossModule signal, got %f", fv.CrossModule)
	}
}

func TestExtract_PlanningTask(t *testing.T) {
	prompt := "plan the migration: 1. backup data 2. run schema changes 3. verify integrity"
	fv := Extract(prompt)

	if fv.Planning < 0.4 {
		t.Errorf("planning task should have high Planning signal, got %f", fv.Planning)
	}
}

func TestExtract_ContextSize(t *testing.T) {
	short := "fix bug"
	long := ""
	for i := 0; i < 100; i++ {
		long += "implement a complex feature with many details and requirements. "
	}

	shortFV := Extract(short)
	longFV := Extract(long)

	if shortFV.ContextSize >= longFV.ContextSize {
		t.Errorf("longer prompt should have higher ContextSize: short=%f, long=%f",
			shortFV.ContextSize, longFV.ContextSize)
	}
}

func TestExtract_AllSignalsNormalized(t *testing.T) {
	prompts := []string{
		"rename variable",
		"implement authentication with JWT tokens and role-based access",
		"fix race condition in concurrent goroutines with mutex",
		"migrate database schema",
		"",
	}

	for _, prompt := range prompts {
		fv := Extract(prompt)
		slice := fv.AsSlice()

		for i, v := range slice {
			if v < 0 || v > 1 {
				t.Errorf("signal %d out of range [0,1]: %f for prompt: %q", i, v, prompt)
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{-1, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.5, 1},
	}

	for _, tt := range tests {
		got := normalize(tt.in)
		if got != tt.want {
			t.Errorf("normalize(%f) = %f, want %f", tt.in, got, tt.want)
		}
	}
}
