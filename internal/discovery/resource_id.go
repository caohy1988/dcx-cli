package discovery

import "strings"

// injectResourceIDs adds a _resource_id field to each item in a list.
// The ID is the primary identifier an agent should pass to follow-on
// commands (e.g., --dataset-id, --instance-id).
//
// Extraction strategy by response shape:
//   - BigQuery: {resource}Reference.{resource}Id (e.g., datasetReference.datasetId)
//   - Spanner/AlloyDB/Looker: "name" field with full path → leaf segment
//   - CloudSQL: "name" field is already the short name
//   - Fallback: "name" or "id" field as-is
// InjectResourceIDByField adds _resource_id to each item in a list by
// extracting a value from the given field path. For full resource paths,
// it extracts the leaf segment. Exported for use by static list commands.
func InjectResourceIDByField(items []interface{}, field string) []interface{} {
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if val, ok := m[field].(string); ok && val != "" {
			if strings.Contains(val, "/") {
				parts := strings.Split(val, "/")
				m["_resource_id"] = parts[len(parts)-1]
			} else {
				m["_resource_id"] = val
			}
		}
	}
	return items
}

// skipResourceID lists resources where a single _resource_id is not sufficient
// to uniquely address the item in follow-on commands (e.g., CloudSQL users
// require both name and host for MySQL).
var skipResourceID = map[string]bool{
	"users": true, // CloudSQL/AlloyDB users may need host or other context
}

func injectResourceIDs(items []interface{}, resource string) []interface{} {
	if skipResourceID[resource] {
		return items
	}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if id := extractResourceID(m, resource); id != "" {
			m["_resource_id"] = id
		}
	}
	return items
}

func extractResourceID(item map[string]interface{}, resource string) string {
	// Strategy 1: BigQuery-style {resource}Reference.{resource}Id
	// e.g., resource="datasets" → look for datasetReference.datasetId
	singular := singularize(resource)
	refKey := singular + "Reference"
	if ref, ok := item[refKey].(map[string]interface{}); ok {
		idKey := singular + "Id"
		if id, ok := ref[idKey].(string); ok {
			return id
		}
	}

	// Strategy 2: "name" field — extract leaf ID from full resource path
	if name, ok := item["name"].(string); ok {
		// Full resource path: "projects/X/instances/Y" → "Y"
		if strings.Contains(name, "/") {
			parts := strings.Split(name, "/")
			return parts[len(parts)-1]
		}
		// Short name (CloudSQL style)
		return name
	}

	// Strategy 3: "id" field
	if id, ok := item["id"].(string); ok {
		return id
	}

	return ""
}

func singularize(resource string) string {
	if strings.HasSuffix(resource, "ses") {
		return resource[:len(resource)-1] // databases → database
	}
	if strings.HasSuffix(resource, "s") {
		return resource[:len(resource)-1]
	}
	return resource
}
