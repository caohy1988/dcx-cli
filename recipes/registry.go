// Package recipes stores workflow recipes embedded into generated skills.
package recipes

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed *.toml
var registryFS embed.FS

// Recipe is a TOML-defined workflow recipe that can be embedded into one or
// more domain SKILL.md files.
type Recipe struct {
	Name        string   `toml:"name"`
	Title       string   `toml:"title"`
	Description string   `toml:"description"`
	Services    []string `toml:"services"`
	Steps       []string `toml:"steps"`
	Cautions    []string `toml:"cautions"`
}

// Load returns every embedded recipe, sorted by name.
func Load() ([]Recipe, error) {
	return LoadFS(registryFS, ".")
}

// LoadFS loads recipe definitions from all *.toml files in dir.
func LoadFS(fsys fs.FS, dir string) ([]Recipe, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading recipes: %w", err)
	}

	var all []Recipe
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		path := entry.Name()
		if dir != "." {
			path = dir + "/" + entry.Name()
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		parsed, err := decodeRecipes(path, data)
		if err != nil {
			return nil, err
		}
		all = append(all, parsed...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})
	return all, nil
}

// ForDomain returns recipes that declare the given service/domain.
func ForDomain(all []Recipe, domain string) []Recipe {
	var result []Recipe
	for _, recipe := range all {
		for _, service := range recipe.Services {
			if service == domain {
				result = append(result, recipe)
				break
			}
		}
	}
	return result
}

func decodeRecipes(path string, data []byte) ([]Recipe, error) {
	var file struct {
		Recipes []Recipe `toml:"recipes"`
	}
	if err := toml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: parsing TOML: %w", path, err)
	}
	if len(file.Recipes) == 0 {
		return nil, fmt.Errorf("%s: no recipes found", path)
	}
	for i, recipe := range file.Recipes {
		if err := validateRecipe(recipe); err != nil {
			return nil, fmt.Errorf("%s: recipe %d: %w", path, i+1, err)
		}
	}
	return file.Recipes, nil
}

func validateRecipe(recipe Recipe) error {
	switch {
	case strings.TrimSpace(recipe.Name) == "":
		return fmt.Errorf("recipe missing name")
	case strings.TrimSpace(recipe.Title) == "":
		return fmt.Errorf("recipe %q missing title", recipe.Name)
	case strings.TrimSpace(recipe.Description) == "":
		return fmt.Errorf("recipe %q missing description", recipe.Name)
	case len(recipe.Services) == 0:
		return fmt.Errorf("recipe %q missing services", recipe.Name)
	case len(recipe.Steps) == 0:
		return fmt.Errorf("recipe %q missing steps", recipe.Name)
	}
	for _, step := range recipe.Steps {
		if !strings.HasPrefix(step, "dcx ") {
			return fmt.Errorf("recipe %q step is not a dcx command: %s", recipe.Name, step)
		}
	}
	return nil
}
