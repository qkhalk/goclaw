package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nextlevelbuilder/goclaw/internal/eval"
)

type evalOptions struct {
	dsn          string
	migrations   string
	dir          string
	suites       []string
	onlyFailures bool
}

// evalCmd exposes the agent evaluation harness (goclaw eval run|list).
// Suites are YAML files under evals/<category>/; see evals/README.md.
func evalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Agent evaluation harness (memory isolation, run resume, tool security)",
	}
	cmd.AddCommand(evalRunCmd())
	cmd.AddCommand(evalListCmd())
	return cmd
}

func evalRunCmd() *cobra.Command {
	var opts evalOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute eval suites and print the score report",
		Long: `Execute eval suites against the evaluation database.

The database DSN resolves in order: --dsn, $TEST_DATABASE_URL, then the
integration-test default (pgvector on localhost:5433). Migrations are applied
automatically. Run from the repo root so the evals/ and migrations/ dirs
resolve, or pass --dir/--migrations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvalSuites(opts)
		},
	}
	cmd.Flags().StringVar(&opts.dsn, "dsn", "", "evaluation database DSN (default $TEST_DATABASE_URL, then localhost:5433)")
	cmd.Flags().StringVar(&opts.migrations, "migrations", "migrations", "migrations directory")
	cmd.Flags().StringVar(&opts.dir, "dir", "evals", "evals root directory")
	cmd.Flags().StringSliceVar(&opts.suites, "suite", nil, "run only these suites (comma-separated)")
	cmd.Flags().BoolVar(&opts.onlyFailures, "failures-only", false, "only print failing cases")
	return cmd
}

func evalListCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered eval suites and case counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := eval.ListSuites(dir)
			if err != nil {
				return err
			}
			for _, s := range reports {
				fmt.Printf("  %-28s driver=%-8s cases=%d\n", s.Suite, s.Driver, s.Total)
			}
			fmt.Printf("\n%d suite(s) under %s\n", len(reports), dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "evals", "evals root directory")
	return cmd
}

func runEvalSuites(opts evalOptions) error {
	env, err := eval.NewEnv(opts.dsn, opts.migrations)
	if err != nil {
		return err
	}
	defer env.Close()

	reports, err := eval.Run(env, eval.RunOptions{
		Dir:         opts.dir,
		Suites:      opts.suites,
		OnlyFailures: opts.onlyFailures,
	})
	if err != nil {
		return err
	}
	pass := eval.WriteReport(os.Stdout, reports, opts.onlyFailures)
	fmt.Println("\n" + eval.SummaryLine(reports))
	if !pass {
		return errors.New("eval finished with failures (see report above)")
	}
	return nil
}
