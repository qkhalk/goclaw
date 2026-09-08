package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nextlevelbuilder/goclaw/internal/workflow"
)

// workflowCmd exposes the YAML workflow DSL tooling. The engine
// (internal/workflow) is the native DAG executor; a workflow file is
// validated here and executable by any gateway wiring that supplies an
// agent runner (native loop, ACP subprocess, remote worker).
func workflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Validate and inspect declarative multi-agent workflows",
	}
	cmd.AddCommand(workflowValidateCmd())
	return cmd
}

func workflowValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file.yaml>",
		Short: "Validate a workflow YAML file against the DAG executor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read workflow: %w", err)
			}
			dag, err := workflow.ParseWorkflowYAML(data, nilRunner{})
			if err != nil {
				return err
			}
			fmt.Printf("workflow %q: valid, %d step(s)\n", dag.Name(), len(dag.Steps()))
			for _, s := range dag.Steps() {
				detail := s.Type.String()
				if len(s.Deps) > 0 {
					detail += fmt.Sprintf(", deps: %s", joinIDs(s.Deps))
				}
				if s.Retry != nil {
					detail += fmt.Sprintf(", retry: %d attempt(s)", s.Retry.MaxAttempts)
				}
				if s.Timeout > 0 {
					detail += fmt.Sprintf(", timeout: %s", s.Timeout)
				}
				fmt.Printf("  - %-16s %s\n", s.ID, detail)
			}
			return nil
		},
	}
}

type nilRunner struct{}

func (nilRunner) RunAgent(context.Context, string, string) (string, error) {
	// Validation never executes steps: ParseWorkflowYAML only builds the DAG.
	return "", nil
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}
