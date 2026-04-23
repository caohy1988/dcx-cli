// Package looker provides a client for the Looker Admin SDK.
//
// This handles API calls to Looker instances for explores and dashboards,
// which are not available through the Google Cloud Discovery pipeline.
package looker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	dcxerrors "github.com/haiyuan-eng-google/dcx-cli/internal/errors"
	"github.com/haiyuan-eng-google/dcx-cli/internal/retry"
)

// Client provides access to the Looker Admin SDK.
type Client struct {
	HTTPClient  *http.Client
	InstanceURL string // e.g. "https://mycompany.looker.com"
	Token       string // Bearer token (Google OAuth or Looker API key)
	MaxRetries  int
}

// NewClient creates a Looker client.
func NewClient(httpClient *http.Client, instanceURL, token string, maxRetries int) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	instanceURL = strings.TrimRight(instanceURL, "/")
	return &Client{
		HTTPClient:  httpClient,
		InstanceURL: instanceURL,
		Token:       token,
		MaxRetries:  maxRetries,
	}
}

// Explore represents a Looker explore.
type Explore struct {
	ModelName   string `json:"model_name"`
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	GroupLabel  string `json:"group_label,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

// ExploresListResult is the output of explores list.
type ExploresListResult struct {
	Items  []Explore `json:"items"`
	Source string    `json:"source"`
}

// Dashboard represents a Looker dashboard.
type Dashboard struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Folder      string `json:"folder,omitempty"`
	ReadOnly    bool   `json:"readonly,omitempty"`
}

// DashboardDetail is the full dashboard response for dashboards get.
type DashboardDetail struct {
	ID          string      `json:"id"`
	Title       string      `json:"title,omitempty"`
	Description string      `json:"description,omitempty"`
	Elements    interface{} `json:"dashboard_elements,omitempty"`
	Filters     interface{} `json:"dashboard_filters,omitempty"`
	Folder      interface{} `json:"folder,omitempty"`
}

// ListExplores retrieves explores from a Looker instance.
func (c *Client) ListExplores(ctx context.Context) (*ExploresListResult, error) {
	url := c.InstanceURL + "/api/4.0/lookml_models"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating explores request: %w", err)
	}
	c.setHeaders(req)

	resp, err := retry.Do(c.HTTPClient, func() (*http.Request, error) { return req, nil }, c.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("Looker API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading explores response: %w", err)
	}

	// Looker returns an array of LookML models, each containing explores.
	var models []struct {
		Name     string `json:"name"`
		Explores []struct {
			Name        string `json:"name"`
			Label       string `json:"label"`
			Description string `json:"description"`
			GroupLabel  string `json:"group_label"`
			Hidden      bool   `json:"hidden"`
		} `json:"explores"`
	}
	if err := json.Unmarshal(body, &models); err != nil {
		return nil, fmt.Errorf("parsing explores response: %w", err)
	}

	var explores []Explore
	for _, model := range models {
		for _, exp := range model.Explores {
			explores = append(explores, Explore{
				ModelName:   model.Name,
				Name:        exp.Name,
				Label:       exp.Label,
				Description: exp.Description,
				GroupLabel:  exp.GroupLabel,
				Hidden:      exp.Hidden,
			})
		}
	}

	return &ExploresListResult{
		Items:  explores,
		Source: "Looker",
	}, nil
}

// GetDashboard retrieves a dashboard by ID.
func (c *Client) GetDashboard(ctx context.Context, dashboardID string) (*DashboardDetail, error) {
	url := c.InstanceURL + "/api/4.0/dashboards/" + dashboardID

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating dashboard request: %w", err)
	}
	c.setHeaders(req)

	resp, err := retry.Do(c.HTTPClient, func() (*http.Request, error) { return req, nil }, c.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("Looker API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading dashboard response: %w", err)
	}

	var dashboard DashboardDetail
	if err := json.Unmarshal(body, &dashboard); err != nil {
		return nil, fmt.Errorf("parsing dashboard response: %w", err)
	}

	return &dashboard, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
}

func readError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var apiErr struct {
		Message string `json:"message"`
	}
	message := fmt.Sprintf("Looker API returned HTTP %d", resp.StatusCode)
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
		message = apiErr.Message
	}
	return &dcxerrors.APIHTTPError{StatusCode: resp.StatusCode, Message: message, RetryAfter: resp.Header.Get("Retry-After")}
}
