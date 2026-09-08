package main

import (
	"fmt"
	"os"

	"github.com/nextlevelbuilder/goclaw/internal/eval"
)

// evalrunner is a minimal CLI that links internal/eval without the full
// gateway binary. Useful on constrained hosts (512MB VPS) where linking the
// whole gateway is too heavy: it runs the same suites via `goclaw eval run`,
// this binary just avoids the gateway's dependency tree at link time.
func main() {
	env, err := eval.NewEnv("", "migrations")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer env.Close()
	reports, err := eval.Run(env, eval.RunOptions{Dir: "evals"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	pass := eval.WriteReport(os.Stdout, reports, false)
	fmt.Println()
	fmt.Println(eval.SummaryLine(reports))
	if !pass {
		os.Exit(1)
	}
}
