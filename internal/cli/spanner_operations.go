package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/haiyuan-eng-google/dcx-cli/internal/auth"
	"github.com/haiyuan-eng-google/dcx-cli/internal/contracts"
	dcxerrors "github.com/haiyuan-eng-google/dcx-cli/internal/errors"
	"github.com/spf13/cobra"
)

const spannerOperationsURL = "https://spanner.googleapis.com/v1/%s"

func (a *App) registerSpannerOperationsCommands() {
	// Find the spanner group command.
	var spannerCmd *cobra.Command
	for _, child := range a.Root.Commands() {
		if child.Name() == "spanner" {
			spannerCmd = child
			break
		}
	}
	if spannerCmd == nil {
		return
	}

	// Find or create operations group.
	var opsCmd *cobra.Command
	for _, child := range spannerCmd.Commands() {
		if child.Name() == "operations" {
			opsCmd = child
			break
		}
	}
	if opsCmd == nil {
		opsCmd = &cobra.Command{
			Use:   "operations",
			Short: "Spanner long-running operation commands",
		}
		spannerCmd.AddCommand(opsCmd)
	}

	a.registerSpannerOpsGet(opsCmd)
	a.registerSpannerOpsWait(opsCmd)
}

func (a *App) registerSpannerOpsGet(parent *cobra.Command) {
	var operationName string

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the status of a Spanner long-running operation",
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			if operationName == "" {
				dcxerrors.Emit(dcxerrors.MissingArgument, "required flag --operation-name is missing", "")
				return nil
			}

			format, err := a.OutputFormat()
			if err != nil {
				dcxerrors.Emit(dcxerrors.InvalidConfig, err.Error(), "")
				return nil
			}

			result, err := fetchSpannerOperation(a.AuthConfig(), operationName)
			if err != nil {
				dcxerrors.EmitAPIError(err)
				return nil
			}

			return a.Render(format, result)
		},
	}

	cmd.Flags().StringVar(&operationName, "operation-name", "", "Full operation resource name (required)")
	parent.AddCommand(cmd)

	a.Registry.Register(contracts.BuildContract(
		"spanner operations get", "spanner",
		"Get the status of a Spanner long-running operation",
		[]contracts.FlagContract{
			{Name: "operation-name", Type: "string", Description: "Full operation resource name", Required: true},
		},
		false, false,
	))
}

func (a *App) registerSpannerOpsWait(parent *cobra.Command) {
	var operationName string
	var timeoutSecs int
	var pollIntervalSecs int

	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait for a Spanner long-running operation to complete",
		Long: `Poll a Spanner long-running operation until it completes or times out.
Quiet during polling — outputs nothing until completion or timeout.

On success: outputs the operation result to stdout.
On timeout: emits a structured error to stderr (exit 2, retryable).`,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			if operationName == "" {
				dcxerrors.Emit(dcxerrors.MissingArgument, "required flag --operation-name is missing", "")
				return nil
			}
			if timeoutSecs <= 0 {
				dcxerrors.Emit(dcxerrors.InvalidConfig, "--timeout must be positive", "")
				return nil
			}
			if pollIntervalSecs <= 0 {
				dcxerrors.Emit(dcxerrors.InvalidConfig, "--poll-interval must be positive", "")
				return nil
			}

			format, err := a.OutputFormat()
			if err != nil {
				dcxerrors.Emit(dcxerrors.InvalidConfig, err.Error(), "")
				return nil
			}

			deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
			interval := time.Duration(pollIntervalSecs) * time.Second

			for {
				result, err := fetchSpannerOperation(a.AuthConfig(), operationName)
				if err != nil {
					dcxerrors.EmitAPIError(err)
					return nil
				}

				done, _ := result["done"].(bool)
				if done {
					return a.Render(format, result)
				}

				if time.Now().Add(interval).After(deadline) {
					dcxerrors.Emit(dcxerrors.InfraError,
						fmt.Sprintf("operation timed out after %ds", timeoutSecs),
						fmt.Sprintf("Operation still running: %s", operationName))
					return nil
				}

				time.Sleep(interval)
			}
		},
	}

	cmd.Flags().StringVar(&operationName, "operation-name", "", "Full operation resource name (required)")
	cmd.Flags().IntVar(&timeoutSecs, "timeout", 300, "Timeout in seconds (default 300)")
	cmd.Flags().IntVar(&pollIntervalSecs, "poll-interval", 2, "Poll interval in seconds (default 2)")
	parent.AddCommand(cmd)

	a.Registry.Register(contracts.BuildContract(
		"spanner operations wait", "spanner",
		"Wait for a Spanner long-running operation to complete",
		[]contracts.FlagContract{
			{Name: "operation-name", Type: "string", Description: "Full operation resource name", Required: true},
			{Name: "timeout", Type: "int", Description: "Timeout in seconds (default 300)"},
			{Name: "poll-interval", Type: "int", Description: "Poll interval in seconds (default 2)"},
		},
		false, false,
	))
}

// fetchSpannerOperation calls GET v1/{name} on the Spanner Operations API.
func fetchSpannerOperation(authCfg auth.Config, operationName string) (map[string]interface{}, error) {
	ctx := context.Background()
	resolved, err := auth.Resolve(ctx, authCfg)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	tok, err := resolved.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("token: %w", err)
	}

	apiURL := fmt.Sprintf(spannerOperationsURL, operationName)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		message := fmt.Sprintf("API returned HTTP %d", resp.StatusCode)
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			message = apiErr.Error.Message
		}
		return nil, &dcxerrors.APIHTTPError{
			StatusCode: resp.StatusCode,
			Message:    message,
			RetryAfter: resp.Header.Get("Retry-After"),
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return result, nil
}
