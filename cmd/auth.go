package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/oauth"
	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate named OAuth subscription accounts (ChatGPT, Claude, Copilot)",
		Long:  "Manage OAuth subscription authentication via the running gateway. Requires the gateway to be running.",
	}
	cmd.AddCommand(authStatusCmd())
	cmd.AddCommand(authLogoutCmd())
	return cmd
}

// gatewayRequest sends an authenticated request to the running gateway.
// Delegates to the shared HTTP client in gateway_http_client.go.
func gatewayRequest(method, path string) (map[string]any, error) {
	return gatewayHTTPDo(method, path, nil)
}

// oauthAuthBase returns the gateway route segment for a provider alias. The
// alias's provider type decides the flow (chatgpt|claude|copilot); unknown
// aliases fall back to the chatgpt routes, which own the legacy default.
func oauthAuthBase(provider string) string {
	base := "/v1/auth/chatgpt"
	if provider != oauth.DefaultProviderName {
		if result, err := gatewayRequest("GET", "/v1/providers?page_size=200"); err == nil {
			if items, ok := result["providers"].([]any); ok {
				for _, it := range items {
					p, ok := it.(map[string]any)
					if !ok {
						continue
					}
					if name, _ := p["name"].(string); name == provider {
						switch t, _ := p["provider_type"].(string); t {
						case "claude_oauth":
							base = "/v1/auth/claude"
						case "copilot_oauth":
							base = "/v1/auth/copilot"
						}
						break
					}
				}
			}
		}
	}
	return base
}

func authStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [provider]",
		Short: "Show OAuth authentication status",
		Long:  "Check if a named OAuth subscription account (ChatGPT, Claude, Copilot) is authenticated on the running gateway.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := resolveOAuthProviderArg(args)
			result, err := gatewayRequest("GET", fmt.Sprintf("%s/%s/status", oauthAuthBase(provider), url.PathEscape(provider)))
			if err != nil {
				return err
			}

			if auth, _ := result["authenticated"].(bool); auth {
				name, _ := result["provider_name"].(string)
				if name == "" {
					name = provider
				}
				fmt.Printf("OAuth account: active (alias: %s)\n", name)
				fmt.Printf("Use model prefix '%s/' in agent config (e.g. %s/<model>).\n", name, name)
			} else {
				fmt.Printf("No OAuth tokens found for alias '%s'.\n", provider)
				fmt.Println("Use the web UI (Providers page) to authenticate this OAuth account.")
			}
			return nil
		},
	}
}

func authLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout [provider]",
		Short: "Disconnect stored OAuth tokens",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := resolveOAuthProviderArg(args)
			_, err := gatewayRequest("POST", fmt.Sprintf("%s/%s/logout", oauthAuthBase(provider), url.PathEscape(provider)))
			if err != nil {
				return err
			}

			fmt.Printf("OAuth account disconnected for alias '%s'.\n", provider)
			return nil
		},
	}
}

func resolveOAuthProviderArg(args []string) string {
	if len(args) == 0 {
		return oauth.DefaultProviderName
	}
	provider := strings.TrimSpace(args[0])
	if provider == "" || provider == "openai" {
		return oauth.DefaultProviderName
	}
	return provider
}
