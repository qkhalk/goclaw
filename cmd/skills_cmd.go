package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

func skillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "List and manage skills",
	}
	cmd.AddCommand(skillsListCmd())
	cmd.AddCommand(skillsShowCmd())
	cmd.AddCommand(skillsEvolveCmd())
	cmd.AddCommand(skillsMetricsCmd())
	cmd.AddCommand(skillsActivityCmd())
	cmd.AddCommand(skillsSuggestionsCmd())
	cmd.AddCommand(skillsDepsCmd())
	cmd.AddCommand(skillsAccessCmd())
	cmd.AddCommand(skillsGrantCmd())
	cmd.AddCommand(skillsRevokeCmd())
	cmd.AddCommand(skillsMarketCmd())
	return cmd
}

func skillsListCmd() *cobra.Command {
	var jsonOutput bool
	var agentID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available skills",
		Run: func(cmd *cobra.Command, args []string) {
			// If --agent specified and gateway is running, use HTTP API
			if agentID != "" && isGatewayReachable() {
				runSkillsListHTTP(agentID, jsonOutput)
				return
			}

			// Fallback: filesystem-based skill listing
			loader := loadSkillsLoader()
			allSkills := loader.ListSkills(context.Background())

			if jsonOutput {
				data, _ := json.MarshalIndent(allSkills, "", "  ")
				fmt.Println(string(data))
				return
			}

			if len(allSkills) == 0 {
				fmt.Println("No skills found.")
				return
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(tw, "NAME\tSOURCE\tDESCRIPTION\n")
			for _, s := range allSkills {
				desc := s.Description
				if runes := []rune(desc); len(runes) > 60 {
					desc = string(runes[:57]) + "..."
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Source, desc)
			}
			tw.Flush()
		},
	}
	cmd.Flags().StringVar(&agentID, "agent", "", "agent ID to list skills for (uses gateway API)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func skillsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show details and content of a skill",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			loader := loadSkillsLoader()
			info, ok := loader.GetSkill(context.Background(), args[0])
			if !ok {
				fmt.Fprintf(os.Stderr, "Skill not found: %s\n", args[0])
				os.Exit(1)
			}
			fmt.Printf("Name:        %s\n", info.Name)
			fmt.Printf("Description: %s\n", info.Description)
			fmt.Printf("Source:      %s\n", info.Source)
			fmt.Printf("Location:    %s\n", info.Path)
			fmt.Println()

			content, ok := loader.LoadSkill(context.Background(), args[0])
			if ok {
				fmt.Println("--- Content ---")
				fmt.Println(content)
			}
		},
	}
}

// runSkillsListHTTP fetches skills for a specific agent from the gateway API.
func runSkillsListHTTP(agentID string, jsonOutput bool) {
	resp, err := gatewayHTTPGet("/v1/agents/" + url.PathEscape(agentID) + "/skills")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return
	}

	raw, _ := json.Marshal(resp["skills"])
	var skills []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &skills); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing skills: %v\n", err)
		os.Exit(1)
	}

	if len(skills) == 0 {
		fmt.Println("No skills found for this agent.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "NAME\tDESCRIPTION\n")
	for _, s := range skills {
		desc := s.Description
		if runes := []rune(desc); len(runes) > 60 {
			desc = string(runes[:57]) + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\n", s.Name, desc)
	}
	tw.Flush()
}

func loadSkillsLoader() *skills.Loader {
	cfgPath := resolveConfigPath()
	cfg, _ := config.Load(cfgPath)
	workspace := config.ExpandHome(cfg.Agents.Defaults.Workspace)
	globalSkillsDir := os.Getenv("GOCLAW_SKILLS_DIR")
	if globalSkillsDir == "" {
		globalSkillsDir = filepath.Join(cfg.ResolvedDataDir(), "skills")
	}
	builtinSkillsDir := os.Getenv("GOCLAW_BUILTIN_SKILLS_DIR")
	if builtinSkillsDir == "" {
		builtinSkillsDir = "/app/bundled-skills"
	}
	return skills.NewLoader(workspace, globalSkillsDir, builtinSkillsDir)
}

// --- Skill market (bundled-skill catalog + on-demand install) ---

// marketEntry mirrors the /v1/skills/market row for CLI output.
type marketEntry struct {
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Category         string   `json:"category"`
	Version          string   `json:"version"`
	Requires         []string `json:"requires"`
	Installed        bool     `json:"installed"`
	InstalledVersion int      `json:"installedVersion"`
	UpdateAvailable  bool     `json:"updateAvailable"`
}

func skillsMarketCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "market",
		Short: "Browse and install bundled skills from the local market",
	}
	cmd.AddCommand(skillsMarketListCmd())
	cmd.AddCommand(skillsMarketInstallCmd())
	return cmd
}

func skillsMarketListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the bundled-skill market catalog",
		Run: func(cmd *cobra.Command, args []string) {
			// Prefer the live gateway: it annotates installed state.
			if isGatewayReachable() {
				runMarketListHTTP(jsonOutput)
				return
			}
			// Offline fallback: scan the bundled dir directly (no installed flags).
			bundledDir := skills.ResolveBundledSkillsDir()
			if bundledDir == "" {
				fmt.Fprintln(os.Stderr, "No bundled skills directory found and gateway is not reachable.")
				os.Exit(1)
			}
			rows, err := skills.BuildMarketCatalog(bundledDir, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			entries := make([]marketEntry, 0, len(rows))
			for _, row := range rows {
				entries = append(entries, marketEntry{
					Slug: row.Slug, Name: row.Name, Description: row.Description,
					Category: row.Category, Version: row.Version, Requires: row.Requires,
				})
			}
			printMarketRows(entries, jsonOutput, false)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func skillsMarketInstallCmd() *cobra.Command {
	var grantAgents []string
	cmd := &cobra.Command{
		Use:   "install <slug>...",
		Short: "Install bundled skills into the running gateway",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if !isGatewayReachable() {
				fmt.Fprintln(os.Stderr, "Gateway is not reachable — start it first (installs require the database).")
				os.Exit(1)
			}
			body := map[string]any{"slugs": args}
			if len(grantAgents) > 0 {
				body["grantAgentIds"] = grantAgents
			}
			resp, err := gatewayHTTPPost("/v1/skills/market/install", body)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			data, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(data))
		},
	}
	cmd.Flags().StringSliceVar(&grantAgents, "grant-agents", nil, "agent IDs to grant the installed skills to")
	return cmd
}

func runMarketListHTTP(jsonOutput bool) {
	resp, err := gatewayHTTPGet("/v1/skills/market")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	raw, _ := json.Marshal(resp["skills"])
	var rows []marketEntry
	if err := json.Unmarshal(raw, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing market catalog: %v\n", err)
		os.Exit(1)
	}
	printMarketRows(rows, jsonOutput, true)
}

func printMarketRows(rows []marketEntry, jsonOutput, withInstalled bool) {
	if jsonOutput {
		data, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(data))
		return
	}
	if len(rows) == 0 {
		fmt.Println("No bundled skills found.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if withInstalled {
		fmt.Fprintf(tw, "SLUG\tCATEGORY\tINSTALLED\tVERSION\tDESCRIPTION\n")
	} else {
		fmt.Fprintf(tw, "SLUG\tCATEGORY\tVERSION\tDESCRIPTION\n")
	}
	for _, row := range rows {
		desc := row.Description
		if runes := []rune(desc); len(runes) > 50 {
			desc = string(runes[:47]) + "..."
		}
		if withInstalled {
			installed := "-"
			if row.Installed {
				installed = fmt.Sprintf("v%d", row.InstalledVersion)
			}
			if row.UpdateAvailable {
				installed += " (update)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.Slug, row.Category, installed, row.Version, desc)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", row.Slug, row.Category, row.Version, desc)
		}
	}
	tw.Flush()
}
