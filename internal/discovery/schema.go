package discovery

import "encoding/json"

// ExtractSchemas parses the schemas section from a raw Discovery doc.
func ExtractSchemas(docJSON []byte) (map[string]interface{}, error) {
	var doc struct {
		Schemas map[string]interface{} `json:"schemas"`
	}
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, err
	}
	if doc.Schemas == nil {
		doc.Schemas = make(map[string]interface{})
	}
	return doc.Schemas, nil
}

// ResolveRefs recursively expands {"$ref": "TypeName"} objects in a schema
// tree using the provided schemas map. Uses a seen set for cycle detection.
func ResolveRefs(value interface{}, schemas map[string]interface{}) interface{} {
	seen := make(map[string]bool)
	return resolveRefsRecursive(value, schemas, seen)
}

func resolveRefsRecursive(value interface{}, schemas map[string]interface{}, seen map[string]bool) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		// Check for objects containing "$ref" — expand the referenced schema
		// and merge any sibling fields (like "description") into the result.
		if ref, ok := v["$ref"].(string); ok {
			if seen[ref] {
				return map[string]interface{}{"$ref": ref, "_circular": true}
			}
			if schema, ok := schemas[ref]; ok {
				seen[ref] = true
				resolved := resolveRefsRecursive(schema, schemas, seen)
				delete(seen, ref)

				// Merge sibling fields (e.g., description) into resolved schema.
				if resolvedMap, ok := resolved.(map[string]interface{}); ok {
					merged := make(map[string]interface{}, len(resolvedMap)+len(v))
					for k, val := range resolvedMap {
						merged[k] = val
					}
					for k, val := range v {
						if k != "$ref" {
							merged[k] = val
						}
					}
					return merged
				}
				return resolved
			}
			return v
		}

		// Recurse into all values.
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = resolveRefsRecursive(val, schemas, seen)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = resolveRefsRecursive(val, schemas, seen)
		}
		return result

	default:
		return value
	}
}
