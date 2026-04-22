package discovery

import "testing"

func TestExtractResourceID_BigQueryReference(t *testing.T) {
	item := map[string]interface{}{
		"datasetReference": map[string]interface{}{
			"datasetId": "my-dataset",
			"projectId": "my-project",
		},
		"kind": "bigquery#dataset",
	}
	id := extractResourceID(item, "datasets")
	if id != "my-dataset" {
		t.Errorf("got %q, want my-dataset", id)
	}
}

func TestExtractResourceID_BigQueryTableReference(t *testing.T) {
	item := map[string]interface{}{
		"tableReference": map[string]interface{}{
			"tableId":   "orders",
			"datasetId": "sales",
			"projectId": "p",
		},
	}
	id := extractResourceID(item, "tables")
	if id != "orders" {
		t.Errorf("got %q, want orders", id)
	}
}

func TestExtractResourceID_SpannerFullPath(t *testing.T) {
	item := map[string]interface{}{
		"name":  "projects/my-project/instances/my-instance",
		"state": "READY",
	}
	id := extractResourceID(item, "instances")
	if id != "my-instance" {
		t.Errorf("got %q, want my-instance", id)
	}
}

func TestExtractResourceID_SpannerDatabase(t *testing.T) {
	item := map[string]interface{}{
		"name":  "projects/p/instances/i/databases/my-db",
		"state": "READY",
	}
	id := extractResourceID(item, "databases")
	if id != "my-db" {
		t.Errorf("got %q, want my-db", id)
	}
}

func TestExtractResourceID_CloudSQLShortName(t *testing.T) {
	item := map[string]interface{}{
		"name": "my-instance",
		"kind": "sql#instance",
	}
	id := extractResourceID(item, "instances")
	if id != "my-instance" {
		t.Errorf("got %q, want my-instance", id)
	}
}

func TestExtractResourceID_FlagsName(t *testing.T) {
	item := map[string]interface{}{
		"name": "max_connections",
		"kind": "sql#flag",
	}
	id := extractResourceID(item, "flags")
	if id != "max_connections" {
		t.Errorf("got %q, want max_connections", id)
	}
}

func TestExtractResourceID_FallbackID(t *testing.T) {
	item := map[string]interface{}{
		"id": "abc123",
	}
	id := extractResourceID(item, "things")
	if id != "abc123" {
		t.Errorf("got %q, want abc123", id)
	}
}

func TestExtractResourceID_NoID(t *testing.T) {
	item := map[string]interface{}{
		"kind": "something",
	}
	id := extractResourceID(item, "things")
	if id != "" {
		t.Errorf("got %q, want empty", id)
	}
}

func TestInjectResourceIDs(t *testing.T) {
	items := []interface{}{
		map[string]interface{}{"name": "projects/p/instances/a"},
		map[string]interface{}{"name": "projects/p/instances/b"},
	}
	result := injectResourceIDs(items, "instances")
	for i, item := range result {
		m := item.(map[string]interface{})
		id, ok := m["_resource_id"].(string)
		if !ok {
			t.Errorf("item[%d] missing _resource_id", i)
		}
		expected := []string{"a", "b"}[i]
		if id != expected {
			t.Errorf("item[%d]._resource_id = %q, want %q", i, id, expected)
		}
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"datasets", "dataset"},
		{"tables", "table"},
		{"instances", "instance"},
		{"databases", "database"},
		{"backups", "backup"},
		{"clusters", "cluster"},
		{"flags", "flag"},
	}
	for _, tt := range tests {
		if got := singularize(tt.input); got != tt.want {
			t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
