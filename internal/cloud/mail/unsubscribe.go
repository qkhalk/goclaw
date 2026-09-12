package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// UnsubscribeMechanism classifies how a message advertises unsubscription
// (RFC 8058 one-click POST, a mailto: loop, or a plain https GET page).
type UnsubscribeMechanism string

const (
	UnsubRFC8058 UnsubscribeMechanism = "rfc8058_post"
	UnsubHTTPGet UnsubscribeMechanism = "https_get"
	UnsubMailto  UnsubscribeMechanism = "mailto"
)

// UnsubscribePlan is the analysis-only result returned by AnalyzeUnsubscribe:
// the agent presents it to the user BEFORE any execution (RFC 8058 §6: a
// receiver MUST NOT POST without user consent).
type UnsubscribePlan struct {
	Mechanism   UnsubscribeMechanism `json:"mechanism"`
	URL         string               `json:"url,omitempty"`
	Mailto      string               `json:"mailto,omitempty"`
	OneClick    bool                 `json:"one_click"`
	ListUnsub   string               `json:"list_unsubscribe_header"`
	Explanation string               `json:"explanation"`
}

// AnalyzeUnsubscribe inspects the List-Unsubscribe headers of a message and
// returns a plan without performing any network call.
func AnalyzeUnsubscribe(detail *MessageDetail) (*UnsubscribePlan, error) {
	header := detail.ListUnsubscribe
	if strings.TrimSpace(header) == "" {
		return nil, errors.New("no List-Unsubscribe header on this message (not a mailing list?)")
	}

	plan := &UnsubscribePlan{ListUnsub: header}

	// Parse comma-separated URL list: <https://a>, <mailto:b>. Prefer https
	// entries; mailto is reported but never auto-executed in v1.
	httpsURL, mailto := "", ""
	for _, part := range splitUnsubscribeList(header) {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "mailto:"); idx >= 0 {
			if mailto == "" {
				if end := strings.IndexAny(part[idx:], " >"); end > 0 {
					mailto = part[idx : idx+end]
				} else {
					mailto = part[idx:]
				}
			}
			continue
		}
		if start := strings.Index(part, "https://"); start >= 0 {
			if httpsURL == "" {
				if end := strings.IndexAny(part[start:], " >"); end > 0 {
					httpsURL = part[start : start+end]
				} else {
					httpsURL = part[start:]
				}
			}
		}
	}

	if httpsURL != "" && strings.EqualFold(strings.TrimSpace(detail.ListUnsubscribePost), "List-Unsubscribe=One-Click") {
		plan.Mechanism = UnsubRFC8058
		plan.URL = httpsURL
		plan.OneClick = true
		plan.Explanation = "Sender supports RFC 8058 one-click unsubscribe: a single POST (no email is sent)."
		return plan, nil
	}
	if httpsURL != "" {
		plan.Mechanism = UnsubHTTPGet
		plan.URL = httpsURL
		plan.Explanation = "Sender offers an unsubscribe web page (a GET visit — you may need to confirm on the page)."
		return plan, nil
	}
	if mailto != "" {
		plan.Mechanism = UnsubMailto
		plan.Mailto = mailto
		plan.Explanation = "Sender only offers email-based unsubscribe. GoClaw v1 does not send email automatically — reply from your mail client or visit the sender's site."
		return plan, nil
	}
	return nil, fmt.Errorf("no actionable unsubscribe mechanism parsed from header: %s", header)
}

// ExecuteUnsubscribe performs the plan's https action (RFC 8058 POST or a
// no-redirect GET). The caller MUST have user consent before calling.
func ExecuteUnsubscribe(ctx context.Context, client *http.Client, plan *UnsubscribePlan) error {
	switch plan.Mechanism {
	case UnsubRFC8058:
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, plan.URL,
			strings.NewReader(url.Values{"List-Unsubscribe": {"One-Click"}}.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("List-Unsubscribe", "One-Click")
		return doUnsubRequest(ctx, client, req)
	case UnsubHTTPGet:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, plan.URL, nil)
		if err != nil {
			return err
		}
		return doUnsubRequest(ctx, client, req)
	case UnsubMailto:
		return errors.New("mailto unsubscribe requires sending email from your account — not supported in v1; agent should inform the user")
	default:
		return errors.New("unknown unsubscribe mechanism")
	}
}

// doUnsubRequest executes the request without following redirects (the
// unsubscribe endpoint must answer 2xx directly; a redirect to a branding
// page is not a confirmation) and never returns page bodies to callers.
func doUnsubRequest(ctx context.Context, client *http.Client, req *http.Request) error {
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unsubscribe request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	slog.Warn("security.mail_unsubscribe", "method", req.Method, "url", req.URL.Host, "status", resp.StatusCode)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("unsubscribe endpoint returned status %d", resp.StatusCode)
}

// splitUnsubscribeList splits on commas outside <> angle brackets.
func splitUnsubscribeList(header string) []string {
	var parts []string
	depth := 0
	current := strings.Builder{}
	for _, r := range header {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}
		current.WriteRune(r)
	}
	parts = append(parts, current.String())
	return parts
}
