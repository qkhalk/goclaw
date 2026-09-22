package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
)

// Yandex Disk provider constants (Yandex OAuth + Disk REST API). Yandex
// grants permissions AT APP LEVEL — the rclone app has "Yandex.Disk REST API"
// (read/write) registered, so NO scope parameter is sent (Yandex rejects
// unknown scope values and the token covers the app's registered
// permissions). Yandex OAuth has no PKCE support: the confidential-client
// secret alone guards the exchange. oauth.yandex.com and oauth.yandex.ru are
// interchangeable; .com matches rclone.
const (
	YandexProvider = "yandex"
	YandexAuthURL  = "https://oauth.yandex.com/authorize"
	YandexTokenURL = "https://oauth.yandex.com/token"
	YandexInfoURL  = "https://login.yandex.ru/info?format=json"
)

// YandexScopesMarker is the observability value stored in the scopes column:
// Yandex has no per-token scope concept (see scopes.go — write access for
// Yandex is registry-based, like the credential providers).
const YandexScopesMarker = "yandex.disk.rest"

// YandexUserInfo is the subset of login.yandex.ru/info we persist. `login`
// is the unique account name (usually the @yandex.* email; some accounts
// expose a phone number — still a valid per-provider upsert key).
type YandexUserInfo struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

// Email resolves the account identity used as the per-provider upsert key.
func (u *YandexUserInfo) Email() string {
	return u.Login
}

// NewYandexTokenConfig builds the x/oauth2 config for the Yandex provider.
// AuthStyleInParams: client credentials go in the POST body (Yandex documents
// form parameters; HTTP Basic is not documented).
func NewYandexTokenConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Endpoint: oauth2.Endpoint{
			AuthURL:   YandexAuthURL,
			TokenURL:  YandexTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

// ExchangeYandexCode swaps an authorization code for tokens. No code_verifier
// param: Yandex OAuth has no PKCE support.
func ExchangeYandexCode(ctx context.Context, cfg *oauth2.Config, code string) (*oauth2.Token, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("cloud: yandex oauth client not configured")
	}
	return cfg.Exchange(ctx, code)
}

// fetchYandexInfo resolves the account identity via login.yandex.ru/info.
func fetchYandexInfo(ctx context.Context, accessToken string) (*YandexUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, YandexInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yandex info status %d", resp.StatusCode)
	}
	var info YandexUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
