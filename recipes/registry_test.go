package recipes

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadEmbeddedRecipes(t *testing.T) {
	all, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(all) < 3 {
		t.Fatalf("Load returned %d recipes, want at least 3", len(all))
	}

	var hasBigQuery, hasSpanner, hasCrossService bool
	for _, recipe := range all {
		if len(recipe.Steps) == 0 {
			t.Fatalf("recipe %s has no steps", recipe.Name)
		}
		for _, step := range recipe.Steps {
			if !strings.HasPrefix(step, "dcx ") {
				t.Fatalf("recipe %s has non-command step: %s", recipe.Name, step)
			}
		}
		services := strings.Join(recipe.Services, ",")
		if services == "bigquery" {
			hasBigQuery = true
		}
		if services == "spanner" {
			hasSpanner = true
		}
		if strings.Contains(services, "bigquery") && strings.Contains(services, "ca") && strings.Contains(services, "spanner") {
			hasCrossService = true
		}
	}

	if !hasBigQuery {
		t.Fatal("missing BigQuery-only recipe")
	}
	if !hasSpanner {
		t.Fatal("missing Spanner-only recipe")
	}
	if !hasCrossService {
		t.Fatal("missing cross-service recipe")
	}
}

func TestForDomain(t *testing.T) {
	all := []Recipe{
		{Name: "a", Services: []string{"bigquery"}},
		{Name: "b", Services: []string{"spanner", "ca"}},
	}

	got := ForDomain(all, "ca")
	if len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("ForDomain(ca) = %+v, want recipe b", got)
	}
}

func TestLoadFSSupportsStandardTOMLFeatures(t *testing.T) {
	fsys := fstest.MapFS{
		"recipes.toml": {
			Data: []byte(`
[[recipes]]
name = "standard-toml" # inline comments should parse
title = "Standard TOML"
description = """
Recipe descriptions may span multiple lines.
"""
services = ["bigquery"]
steps = [
  "dcx datasets list --format json", # comments inside arrays should parse
]
cautions = ["Keep output bounded."]
`),
		},
	}

	all, err := LoadFS(fsys, ".")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("LoadFS returned %d recipes, want 1", len(all))
	}
	if !strings.Contains(all[0].Description, "span multiple lines") {
		t.Fatalf("multiline description was not decoded: %q", all[0].Description)
	}
}
