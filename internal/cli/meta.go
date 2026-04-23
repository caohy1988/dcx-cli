package cli

import (
	"fmt"
	"strings"

	"github.com/haiyuan-eng-google/dcx-cli/assets"
	"github.com/haiyuan-eng-google/dcx-cli/internal/contracts"
	"github.com/haiyuan-eng-google/dcx-cli/internal/discovery"
	dcxerrors "github.com/haiyuan-eng-google/dcx-cli/internal/errors"
	"github.com/haiyuan-eng-google/dcx-cli/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) addMetaCommands() {
	metaCmd := &cobra.Command{
		Use:   "meta",
		Short: "Introspection commands for the dcx contract",
	}

	metaCmd.AddCommand(a.metaCommandsCmd())
	metaCmd.AddCommand(a.metaDescribeCmd())
	metaCmd.AddCommand(a.metaSchemaCmd())

	a.Root.AddCommand(metaCmd)
}

func (a *App) metaCommandsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List all registered commands and their domains",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := a.OutputFormat()
			if err != nil {
				dcxerrors.Emit(dcxerrors.InvalidConfig, err.Error(), "Use --format with: "+strings.Join(output.FormatNames(), ", "))
				return nil // unreachable after Emit exits
			}

			summaries := a.Registry.ListCommands()

			switch format {
			case output.Table:
				// Convert to []map[string]interface{} for table rendering.
				rows := make([]map[string]interface{}, len(summaries))
				for i, s := range summaries {
					rows[i] = map[string]interface{}{
						"command":     s.Command,
						"domain":      s.Domain,
						"description": s.Description,
					}
				}
				return a.Render(format, rows)
			default:
				return a.Render(format, summaries)
			}
		},
	}
}

func (a *App) metaDescribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe [command...]",
		Short: "Show the full contract for a given command",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := a.OutputFormat()
			if err != nil {
				dcxerrors.Emit(dcxerrors.InvalidConfig, err.Error(), "Use --format with: "+strings.Join(output.FormatNames(), ", "))
				return nil
			}

			commandPath := strings.Join(args, " ")
			contract, err := a.Registry.Describe(commandPath)
			if err != nil {
				dcxerrors.Emit(dcxerrors.UnknownCommand, err.Error(), "Run 'dcx meta commands' to see available commands")
				return nil
			}

			switch format {
			case output.Table:
				// For table, show contract as key-value pairs.
				kv := map[string]interface{}{
					"command":          contract.Command,
					"contract_version": contract.ContractVersion,
					"domain":           contract.Domain,
					"description":      contract.Description,
					"supports_dry_run": contract.SupportsDryRun,
					"is_mutation":      contract.IsMutation,
				}
				return a.Render(format, kv)
			default:
				return a.Render(format, contract)
			}
		},
	}
}

func (a *App) metaSchemaCmd() *cobra.Command {
	var resolveRefs bool

	cmd := &cobra.Command{
		Use:   "schema [command...] or [service.TypeName]",
		Short: "Show request/response schema for a command or type",
		Long: `Show the JSON schema for a command's request/response body, or
look up a specific schema type from a Discovery document.

Method lookup:
  dcx meta schema datasets insert
  dcx meta schema spanner databases create

Type lookup:
  dcx meta schema bigquery.Dataset
  dcx meta schema spanner.CreateDatabaseRequest

Use --resolve-refs to expand $ref pointers inline.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formatVal, err := a.OutputFormat()
			if err != nil {
				dcxerrors.Emit(dcxerrors.InvalidConfig, err.Error(), "")
				return nil
			}

			// Type lookup: single arg with a dot (e.g., "bigquery.Dataset").
			if len(args) == 1 && strings.Contains(args[0], ".") {
				return a.schemaTypeLookup(args[0], resolveRefs, formatVal)
			}

			// Method lookup: command path (e.g., "datasets insert").
			return a.schemaMethodLookup(args, resolveRefs, formatVal)
		},
	}

	cmd.Flags().BoolVar(&resolveRefs, "resolve-refs", false, "Expand $ref pointers inline")

	a.Registry.Register(contracts.BuildContract(
		"meta schema", "meta",
		"Show request/response schema for a command or type",
		[]contracts.FlagContract{
			{Name: "resolve-refs", Type: "bool", Description: "Expand $ref pointers inline"},
		},
		false, false,
	))

	return cmd
}

func (a *App) schemaMethodLookup(args []string, resolveRefs bool, format output.Format) error {
	commandPath := strings.Join(args, " ")

	// Strip optional "dcx " prefix so both "datasets insert" and
	// "dcx datasets insert" (copied from meta commands output) work.
	commandPath = strings.TrimPrefix(commandPath, "dcx ")

	// Find the command in the registry to get the domain.
	contract, err := a.Registry.Describe(commandPath)
	if err != nil {
		dcxerrors.Emit(dcxerrors.UnknownCommand, err.Error(), "Run 'dcx meta commands' to see available commands")
		return nil
	}

	// Load Discovery doc for this domain.
	docJSON, err := assets.DiscoveryDocForDomain(contract.Domain)
	if err != nil {
		dcxerrors.Emit(dcxerrors.InvalidConfig,
			fmt.Sprintf("no schema available for domain %q (not Discovery-driven)", contract.Domain),
			"Schema lookup is supported for Discovery-driven commands only")
		return nil
	}

	// Parse the doc to find the matching GeneratedCommand.
	svc := discovery.ConfigForDomain(contract.Domain)
	if svc == nil {
		dcxerrors.Emit(dcxerrors.InvalidConfig, fmt.Sprintf("no service config for domain %q", contract.Domain), "")
		return nil
	}

	commands, err := discovery.Parse(docJSON, svc)
	if err != nil {
		dcxerrors.Emit(dcxerrors.Internal, fmt.Sprintf("parsing Discovery doc: %v", err), "")
		return nil
	}

	var matched *discovery.GeneratedCommand
	for i := range commands {
		if commands[i].CommandPath == commandPath {
			matched = &commands[i]
			break
		}
	}
	if matched == nil {
		// Command exists in registry but not in Discovery doc — it's a static command.
		dcxerrors.Emit(dcxerrors.InvalidConfig,
			fmt.Sprintf("command %q is not Discovery-driven; schema introspection is not available", commandPath),
			"Schema lookup is supported for Discovery-generated commands only")
		return nil
	}

	// Extract schemas from the doc.
	schemas, err := discovery.ExtractSchemas(docJSON)
	if err != nil {
		dcxerrors.Emit(dcxerrors.Internal, fmt.Sprintf("extracting schemas: %v", err), "")
		return nil
	}

	// Build result.
	result := map[string]interface{}{
		"command": contract.Command,
		"domain":  contract.Domain,
	}

	if matched.Method.RequestRef != "" {
		result["request_schema"] = matched.Method.RequestRef
		if schema, ok := schemas[matched.Method.RequestRef]; ok {
			if resolveRefs {
				schema = discovery.ResolveRefs(schema, schemas)
			}
			result["request"] = schema
		}
	} else {
		result["request_schema"] = nil
	}

	if matched.Method.ResponseRef != "" {
		result["response_schema"] = matched.Method.ResponseRef
		if schema, ok := schemas[matched.Method.ResponseRef]; ok {
			if resolveRefs {
				schema = discovery.ResolveRefs(schema, schemas)
			}
			result["response"] = schema
		}
	} else {
		result["response_schema"] = nil
	}

	return a.Render(format, result)
}

func (a *App) schemaTypeLookup(path string, resolveRefs bool, format output.Format) error {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		dcxerrors.Emit(dcxerrors.InvalidConfig, "type lookup format: service.TypeName (e.g., bigquery.Dataset)", "")
		return nil
	}

	domain := parts[0]
	typeName := parts[1]

	docJSON, err := assets.DiscoveryDocForDomain(domain)
	if err != nil {
		dcxerrors.Emit(dcxerrors.InvalidConfig,
			fmt.Sprintf("no Discovery doc for domain %q", domain),
			"Type lookup is supported for Discovery-driven domains: bigquery, spanner, alloydb, cloudsql, looker")
		return nil
	}

	schemas, err := discovery.ExtractSchemas(docJSON)
	if err != nil {
		dcxerrors.Emit(dcxerrors.Internal, fmt.Sprintf("extracting schemas: %v", err), "")
		return nil
	}

	schema, ok := schemas[typeName]
	if !ok {
		dcxerrors.Emit(dcxerrors.NotFound,
			fmt.Sprintf("schema %q not found in %s Discovery doc", typeName, domain), "")
		return nil
	}

	if resolveRefs {
		schema = discovery.ResolveRefs(schema, schemas)
	}

	result := map[string]interface{}{
		"schema_name": typeName,
		"service":     domain,
		"schema":      schema,
	}

	return a.Render(format, result)
}
