package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nextlevelbuilder/goclaw/internal/mcp/scaffold"
)

// mcpCmd groups MCP tool-server authoring helpers (the mcp/ catalog).
func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Author MCP tool servers for the Tool Store catalog (mcp/)",
	}
	cmd.AddCommand(mcpNewCmd())
	return cmd
}

// mcpNewCmd scaffolds mcp/<name>/ — manifest, working zero-dependency server
// skeleton, README, and a runnable smoke test.
func mcpNewCmd() *cobra.Command {
	var (
		runtime     string
		display     string
		description string
		category    string
		out         string
		noSmoke     bool
	)
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new MCP tool server under mcp/<name>",
		Args:  cobra.ExactArgs(1),
		Example: `  goclaw mcp new image-utils
  goclaw mcp new pdf-merge --runtime python --display "PDF Merge" \
    --description "Merge PDF files for agents" --category files`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outRoot := out
			if outRoot == "" {
				root, err := findRepoRoot()
				if err != nil {
					return err
				}
				outRoot = filepath.Join(root, "mcp")
			}
			w, err := scaffold.Generate(scaffold.Options{
				Name:        args[0],
				DisplayName: display,
				Description: description,
				Category:    category,
				Runtime:     runtime,
				OutRoot:     outRoot,
			})
			if err != nil {
				return err
			}
			fmt.Printf("created %s (%s runtime):\n", w.Dir, runtime)
			for _, f := range w.Files {
				fmt.Printf("  %s\n", f)
			}

			if noSmoke {
				fmt.Println("\nnext: edit src/, then run the smoke test yourself:")
				fmt.Printf("  cd %s && %s\n", w.Dir, strings.Join(w.SmokeCmd, " "))
				return nil
			}
			fmt.Println("\nrunning smoke test…")
			argv := append([]string{}, w.SmokeCmd[0:len(w.SmokeCmd)-1]...)
			argv = append(argv, filepath.Join(w.Dir, w.SmokeCmd[len(w.SmokeCmd)-1]))
			smoke := exec.Command(argv[0], argv[1:]...)
			smoke.Stdout = os.Stdout
			smoke.Stderr = os.Stderr
			if err := smoke.Run(); err != nil {
				return fmt.Errorf("smoke test failed — fix src/ before shipping: %w", err)
			}
			fmt.Println("\nnext: commit this folder and cut a release tag — the dynamic")
			fmt.Println("catalog delivers it to every install within ~15 minutes.")
			return nil
		},
	}
	cmd.Flags().StringVar(&runtime, "runtime", "node", "server runtime: node or python")
	cmd.Flags().StringVar(&display, "display", "", "display name (default: title-cased slug)")
	cmd.Flags().StringVar(&description, "description", "", "one-sentence description for the Store card")
	cmd.Flags().StringVar(&category, "category", "tools", "Store card category")
	cmd.Flags().StringVar(&out, "out", "", "output mcp/ root (default: this repo's mcp/)")
	cmd.Flags().BoolVar(&noSmoke, "no-smoke", false, "skip running the smoke test after generating")
	return cmd
}

// findRepoRoot walks up to the checkout containing go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not inside a Go module — run from the goclaw checkout or pass --out")
		}
		dir = parent
	}
}
