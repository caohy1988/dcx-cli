package discovery

import (
	"os"
	"strings"
	"testing"
)

func loadBigQueryDoc(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../assets/bigquery_v2_discovery.json")
	if err != nil {
		t.Fatalf("reading discovery doc: %v", err)
	}
	return data
}

func TestValidateTopLevel_MissingRequired(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{"friendlyName": "test"}`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	if len(errors) == 0 {
		t.Fatal("expected validation errors for missing required field")
	}

	found := false
	for _, e := range errors {
		if e.Field == "datasetReference" && strings.Contains(e.Message, "required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error for missing datasetReference, got: %v", errors)
	}
}

func TestValidateTopLevel_TypeMismatch(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{"datasetReference": {"datasetId": "test"}, "friendlyName": 123}`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	if len(errors) == 0 {
		t.Fatal("expected validation error for type mismatch")
	}

	found := false
	for _, e := range errors {
		if e.Field == "friendlyName" && strings.Contains(e.Message, "expects string") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected type mismatch on friendlyName, got: %v", errors)
	}
}

func TestValidateTopLevel_ValidBody(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{"datasetReference": {"datasetId": "test"}, "friendlyName": "My Dataset"}`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	if len(errors) != 0 {
		t.Errorf("expected no errors for valid body, got: %v", errors)
	}
}

func TestValidateTopLevel_BooleanType(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{"datasetReference": {"datasetId": "test"}, "isCaseInsensitive": "not-a-bool"}`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	found := false
	for _, e := range errors {
		if e.Field == "isCaseInsensitive" && strings.Contains(e.Message, "expects boolean") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected type mismatch on boolean field, got: %v", errors)
	}
}

func TestValidateTopLevel_RefFieldGivenNonObject(t *testing.T) {
	doc := loadBigQueryDoc(t)

	tests := []struct {
		name string
		body string
	}{
		{"null", `{"datasetReference": null}`},
		{"string", `{"datasetReference": "not-object"}`},
		{"number", `{"datasetReference": 42}`},
		{"array", `{"datasetReference": [1,2]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateTopLevelProperties([]byte(tt.body), "Dataset", doc)
			found := false
			for _, e := range errors {
				if e.Field == "datasetReference" && strings.Contains(e.Message, "expects object") {
					found = true
				}
			}
			if !found {
				t.Errorf("expected type error for $ref field given %s, got: %v", tt.name, errors)
			}
		})
	}
}

func TestValidateTopLevel_RefFieldGivenValidObject(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{"datasetReference": {"datasetId": "test"}}`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	for _, e := range errors {
		if e.Field == "datasetReference" {
			t.Errorf("valid object should not error on $ref field, got: %v", e)
		}
	}
}

func TestValidateTopLevel_UnknownSchema(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{"anything": "goes"}`)

	errors := ValidateTopLevelProperties(body, "DoesNotExist", doc)
	if errors != nil {
		t.Errorf("unknown schema should return nil (skip), got: %v", errors)
	}
}

func TestValidateTopLevel_InvalidJSON(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`not json`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	if len(errors) != 1 || !strings.Contains(errors[0].Message, "not a valid JSON object") {
		t.Errorf("expected JSON parse error, got: %v", errors)
	}
}

func TestValidateTopLevel_EmptyBody(t *testing.T) {
	doc := loadBigQueryDoc(t)
	body := []byte(`{}`)

	errors := ValidateTopLevelProperties(body, "Dataset", doc)
	// Should fail for missing required datasetReference.
	if len(errors) == 0 {
		t.Fatal("expected error for empty body missing required fields")
	}
}

func TestFormatValidationErrors(t *testing.T) {
	errors := []ValidationError{
		{Field: "name", Message: "required field is missing"},
		{Field: "age", Message: "expects number, got string"},
	}
	msg := FormatValidationErrors(errors)
	if !strings.Contains(msg, "body validation failed:") {
		t.Errorf("missing prefix: %s", msg)
	}
	if !strings.Contains(msg, "name: required field is missing") {
		t.Errorf("missing name error: %s", msg)
	}
	if !strings.Contains(msg, "age: expects number, got string") {
		t.Errorf("missing age error: %s", msg)
	}
}

func TestCheckType(t *testing.T) {
	tests := []struct {
		value    interface{}
		expected string
		wantErr  bool
	}{
		{"hello", "string", false},
		{42.0, "string", true},
		{42.0, "number", false},
		{42.0, "integer", false},
		{"42", "integer", true},
		{true, "boolean", false},
		{"true", "boolean", true},
		{[]interface{}{1}, "array", false},
		{"[1]", "array", true},
		{map[string]interface{}{}, "object", false},
		{"object", "object", true},
	}
	for _, tt := range tests {
		err := checkType("test", tt.value, tt.expected)
		if (err != nil) != tt.wantErr {
			t.Errorf("checkType(%v, %q) err=%v, wantErr=%v", tt.value, tt.expected, err, tt.wantErr)
		}
	}
}
