package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/haiyuan-eng-google/dcx-cli/internal/contracts"
	"github.com/haiyuan-eng-google/dcx-cli/recipes"
)

func TestRenderSkillEmbedsWorkflows(t *testing.T) {
	cmds := []*contracts.CommandContract{
		contracts.BuildContract("datasets list", "bigquery", "List datasets", nil, false, false),
		contracts.BuildContract("tables get", "bigquery", "Get table", []contracts.FlagContract{
			{Name: "table-id", Type: "string", Description: "Table ID", Required: true},
		}, false, false),
	}
	domainRecipes := []recipes.Recipe{{
		Name:        "bigquery-table-inventory",
		Title:       "Inventory and inspect BigQuery tables",
		Description: "List datasets and inspect a table.",
		Services:    []string{"bigquery"},
		Steps: []string{
			"dcx datasets list --project-id $PROJECT_ID --format json",
			"dcx tables get --project-id $PROJECT_ID --dataset-id $DATASET_ID --table-id $TABLE_ID --format json",
		},
		Cautions: []string{"Keep output bounded."},
	}}

	rendered := renderSkill("bigquery", cmds, domainRecipes)
	for _, want := range []string{
		"## Workflows",
		"### Inventory and inspect BigQuery tables",
		"1. `dcx datasets list --project-id $PROJECT_ID --format json`",
		"Cautions:",
		"## Flag reference",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered skill missing %q:\n%s", want, rendered)
		}
	}
}

func TestEmbeddedRecipeStepsReferenceRegisteredCommands(t *testing.T) {
	app := NewApp()
	allRecipes, err := recipes.Load()
	if err != nil {
		t.Fatalf("recipes.Load: %v", err)
	}

	for _, recipe := range allRecipes {
		for _, step := range recipe.Steps {
			contract, args, ok := stepContractAndArgs(app, step)
			if !ok {
				t.Fatalf("recipe %s step does not reference a registered command: %s", recipe.Name, step)
			}
			if err := validateStepFlags(contract, args); err != nil {
				t.Fatalf("recipe %s step has invalid flags: %v\nstep: %s", recipe.Name, err, step)
			}
		}
	}
}

func stepContractAndArgs(app *App, step string) (*contracts.CommandContract, []string, bool) {
	parts := strings.Fields(step)
	for i := len(parts); i >= 2; i-- {
		candidate := strings.Join(parts[:i], " ")
		if contract, ok := app.Registry.Get(candidate); ok {
			return contract, parts[i:], true
		}
	}
	return nil, nil, false
}

func validateStepFlags(contract *contracts.CommandContract, args []string) error {
	allowed := make(map[string]bool, len(contract.Flags))
	for _, flag := range contract.Flags {
		allowed[flag.Name] = true
	}
	if contract.SupportsDryRun {
		allowed["dry-run"] = true
	}

	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		if name == "" {
			continue
		}
		if !allowed[name] {
			return fmt.Errorf("%s does not define --%s", contract.Command, name)
		}
	}
	return nil
}
