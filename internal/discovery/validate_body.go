package discovery

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationError describes a single body validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) String() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidateTopLevelProperties checks a JSON body against a Discovery schema's
// top-level properties. Validates:
//   - Required fields are present (detected via "Required." in description)
//   - Primitive type matches (string, number/integer, boolean, array, object)
//
// Does NOT validate nested $ref objects, array items, enum values, or
// additionalProperties. Use --no-validate to skip entirely.
//
// Returns nil if valid, or a list of validation errors.
func ValidateTopLevelProperties(body []byte, schemaName string, docJSON []byte) []ValidationError {
	schemas, err := ExtractSchemas(docJSON)
	if err != nil {
		return nil // can't validate, let API handle it
	}

	schemaRaw, ok := schemas[schemaName]
	if !ok {
		return nil // unknown schema, skip
	}

	schemaMap, ok := schemaRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		return nil // no properties defined
	}

	// Parse the body.
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return []ValidationError{{Message: "body is not a valid JSON object"}}
	}

	var errors []ValidationError

	// Check required fields and type mismatches.
	for propName, propRaw := range properties {
		prop, ok := propRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// Required check: description starts with "Required."
		desc, _ := prop["description"].(string)
		isRequired := strings.HasPrefix(strings.TrimSpace(desc), "Required.")

		value, exists := bodyMap[propName]

		if isRequired && !exists {
			errors = append(errors, ValidationError{
				Field:   propName,
				Message: "required field is missing",
			})
			continue
		}

		if !exists {
			continue
		}

		// Type check for present fields.
		expectedType, _ := prop["type"].(string)
		if expectedType == "" {
			// $ref properties are expected to be objects.
			if _, hasRef := prop["$ref"]; hasRef {
				expectedType = "object"
			} else {
				continue // untyped, skip
			}
		}

		if err := checkType(propName, value, expectedType); err != nil {
			errors = append(errors, *err)
		}
	}

	return errors
}

// FormatValidationErrors produces a single error message from a list of
// validation errors, suitable for INVALID_CONFIG emission.
func FormatValidationErrors(errors []ValidationError) string {
	parts := make([]string, len(errors))
	for i, e := range errors {
		parts[i] = e.String()
	}
	return "body validation failed: " + strings.Join(parts, "; ")
}

// checkType validates that a JSON value matches the expected Discovery type.
// integer and number both map to numeric (JSON float64).
func checkType(field string, value interface{}, expectedType string) *ValidationError {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expects string, got %s", jsonTypeName(value))}
		}
	case "integer", "number":
		if _, ok := value.(float64); !ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expects %s, got %s", expectedType, jsonTypeName(value))}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expects boolean, got %s", jsonTypeName(value))}
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expects array, got %s", jsonTypeName(value))}
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expects object, got %s", jsonTypeName(value))}
		}
	}
	return nil
}

func jsonTypeName(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}
