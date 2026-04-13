package ca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/haiyuan-eng-google/dcx-cli/internal/profiles"
)

const (
	// CA Chat API endpoint (BigQuery DataAgent).
	chatAPIURL = "https://datacatalog.googleapis.com/v1/projects/%s/locations/%s/dataAgents:chat"

	// CA QueryData API endpoint (Spanner, AlloyDB, Cloud SQL).
	queryDataAPIURL = "https://datacatalog.googleapis.com/v1/projects/%s/locations/%s:queryData"
)

// Client provides access to the Conversational Analytics APIs.
type Client struct {
	HTTPClient *http.Client
}

// NewClient creates a CA client.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{HTTPClient: httpClient}
}

// Ask routes a question to the appropriate CA API based on profile source type.
func (c *Client) Ask(ctx context.Context, token string, profile *profiles.Profile, question, agent, tables string) (*AskResult, error) {
	if profile != nil && profile.IsQueryDataSource() {
		return c.askQueryData(ctx, token, profile, question)
	}
	return c.askChat(ctx, token, profile, question, agent, tables)
}

// askChat sends a question to the CA Chat API (BigQuery/Looker DataAgent).
func (c *Client) askChat(ctx context.Context, token string, profile *profiles.Profile, question, agent, tables string) (*AskResult, error) {
	projectID := ""
	location := "us" // default location for CA
	if profile != nil {
		projectID = profile.Project
		if profile.Location != "" {
			location = profile.Location
		}
	}
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required for ca ask")
	}

	url := fmt.Sprintf(chatAPIURL, projectID, location)

	reqBody := map[string]interface{}{
		"question": question,
	}
	if agent != "" {
		reqBody["agent"] = agent
	}
	if tables != "" {
		reqBody["tables"] = tables
	}
	// Looker profile: pass explores context.
	if profile != nil && profile.SourceType == profiles.Looker {
		if profile.LookerInstanceURL != "" {
			reqBody["looker_instance_url"] = profile.LookerInstanceURL
		}
		if len(profile.LookerExplores) > 0 {
			reqBody["looker_explores"] = profile.LookerExplores
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chat API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readAPIError(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading chat response: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing chat response: %w", err)
	}

	source := "BigQuery"
	if profile != nil && profile.SourceType == profiles.Looker {
		source = "Looker"
	}

	return &AskResult{
		Question:    chatResp.Question,
		SQL:         chatResp.SQL,
		Results:     chatResp.Results,
		Explanation: chatResp.Explanation,
		Source:      source,
		Agent:       chatResp.Agent,
	}, nil
}

// askQueryData sends a question to the CA QueryData API (Spanner/AlloyDB/CloudSQL).
func (c *Client) askQueryData(ctx context.Context, token string, profile *profiles.Profile, question string) (*AskResult, error) {
	if profile.Project == "" {
		return nil, fmt.Errorf("project is required in profile")
	}

	location := profile.Location
	if location == "" {
		location = "us"
	}

	url := fmt.Sprintf(queryDataAPIURL, profile.Project, location)

	reqBody := QueryDataRequest{
		Question:   question,
		ProjectID:  profile.Project,
		SourceType: string(profile.SourceType),
		Location:   profile.Location,
		InstanceID: profile.InstanceID,
		DatabaseID: profile.DatabaseID,
		ClusterID:  profile.ClusterID,
		DBType:     profile.DBType,
	}
	if profile.ContextSetID != "" {
		reqBody.AgentContextReference = profile.ContextSetID
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling querydata request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating querydata request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querydata API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readAPIError(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading querydata response: %w", err)
	}

	var qdResp QueryDataResponse
	if err := json.Unmarshal(respBody, &qdResp); err != nil {
		return nil, fmt.Errorf("parsing querydata response: %w", err)
	}

	return &AskResult{
		Question:    qdResp.Question,
		SQL:         qdResp.SQL,
		Results:     qdResp.Results,
		Explanation: qdResp.Explanation,
		Source:      sourceName(profile.SourceType),
	}, nil
}

// AskQueryDataRaw executes a raw SQL-like question via QueryData and returns
// the raw response. Used by database helpers (schema describe, databases list).
func (c *Client) AskQueryDataRaw(ctx context.Context, token string, profile *profiles.Profile, question string) (map[string]interface{}, error) {
	if profile.Project == "" {
		return nil, fmt.Errorf("project is required in profile")
	}

	location := profile.Location
	if location == "" {
		location = "us"
	}

	url := fmt.Sprintf(queryDataAPIURL, profile.Project, location)

	reqBody := QueryDataRequest{
		Question:   question,
		ProjectID:  profile.Project,
		SourceType: string(profile.SourceType),
		Location:   profile.Location,
		InstanceID: profile.InstanceID,
		DatabaseID: profile.DatabaseID,
		ClusterID:  profile.ClusterID,
		DBType:     profile.DBType,
	}
	if profile.ContextSetID != "" {
		reqBody.AgentContextReference = profile.ContextSetID
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling querydata request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating querydata request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querydata API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readAPIError(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading querydata response: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parsing querydata response: %w", err)
	}
	return raw, nil
}

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var apiErr struct {
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	message := fmt.Sprintf("API returned HTTP %d", resp.StatusCode)
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
		message = apiErr.Error.Message
	}
	return fmt.Errorf("%s", message)
}

func sourceName(st profiles.SourceType) string {
	switch st {
	case profiles.BigQuery:
		return "BigQuery"
	case profiles.Spanner:
		return "Spanner"
	case profiles.AlloyDB:
		return "AlloyDB"
	case profiles.CloudSQL:
		return "Cloud SQL"
	case profiles.Looker:
		return "Looker"
	default:
		return string(st)
	}
}
