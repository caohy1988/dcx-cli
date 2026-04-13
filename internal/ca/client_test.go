package ca

import (
	"testing"

	"github.com/haiyuan-eng-google/dcx-cli/internal/profiles"
)

func TestSourceName(t *testing.T) {
	tests := []struct {
		st   profiles.SourceType
		want string
	}{
		{profiles.BigQuery, "BigQuery"},
		{profiles.Spanner, "Spanner"},
		{profiles.AlloyDB, "AlloyDB"},
		{profiles.CloudSQL, "Cloud SQL"},
		{profiles.Looker, "Looker"},
	}
	for _, tt := range tests {
		got := sourceName(tt.st)
		if got != tt.want {
			t.Errorf("sourceName(%s) = %s, want %s", tt.st, got, tt.want)
		}
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient(nil)
	if c.HTTPClient == nil {
		t.Error("NewClient(nil) should set default HTTP client")
	}
}

func TestProfileIsQueryDataSource(t *testing.T) {
	tests := []struct {
		st   profiles.SourceType
		want bool
	}{
		{profiles.Spanner, true},
		{profiles.AlloyDB, true},
		{profiles.CloudSQL, true},
		{profiles.BigQuery, false},
		{profiles.Looker, false},
	}
	for _, tt := range tests {
		p := profiles.Profile{SourceType: tt.st}
		if got := p.IsQueryDataSource(); got != tt.want {
			t.Errorf("Profile{%s}.IsQueryDataSource() = %v, want %v", tt.st, got, tt.want)
		}
	}
}
