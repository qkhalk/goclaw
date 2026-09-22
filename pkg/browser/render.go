package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// renderSettleWindow is how long the rendered page must stay quiet (no
// document mutations) before the DOM is read — same settle the tab-open path
// uses so late hydration lands before we snapshot.
const renderSettleWindow = 300 * time.Millisecond

// RenderHTML loads rawURL with full JavaScript execution and returns the
// settled DOM plus the post-redirect location and title. It implements
// tools.PageRenderer — the headless fallback the web_browse relay uses when
// the plain HTTP fetch came back as a JS-only shell or bot challenge
// (WebBrowseTool.maybeRender decides; any error here keeps the original
// fetch, so this path must fail loud, not guess).
//
// The render runs in a throwaway incognito context (same isolation primitive
// as the tenant contexts) that never joins m.pages: it does not count
// against the per-tenant tab budget, scoped tenants never list it as a
// phantom tab (unowned targets are filtered), and the context is disposed
// when the call returns — including on ctx cancel mid-render. Chrome only:
// Lightpanda's JS engine cannot honor the full-render promise, so it errors
// and the caller falls back to the fetch.
func (m *Manager) RenderHTML(ctx context.Context, rawURL string) (*tools.PageRenderResult, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("render: empty url")
	}

	m.mu.Lock()
	if !m.isRunningLocked() {
		m.mu.Unlock()
		return nil, fmt.Errorf("browser not running")
	}
	if m.backend == BackendLightpanda {
		m.mu.Unlock()
		return nil, fmt.Errorf("render fallback requires the chrome backend")
	}
	renderBrowser, err := m.browser.Incognito()
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("render context: %w", err)
	}
	defer renderBrowser.Close()

	page, err := renderBrowser.Page(proto.TargetCreateTarget{URL: rawURL})
	if err != nil {
		return nil, fmt.Errorf("render page: %w", err)
	}
	// Close the page early if the caller gives up — the blocking WaitStable
	// CDP call does not observe ctx cancellation on its own (same watchdog
	// as OpenTab).
	stopWatchdog := watchPageClose(ctx, page)
	defer stopWatchdog()
	if err := page.WaitStable(renderSettleWindow); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("wait stable: %w", err)
	}
	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("read rendered html: %w", err)
	}
	res := &tools.PageRenderResult{HTML: html}
	if info, infoErr := page.Info(); infoErr == nil && info != nil {
		res.FinalURL = info.URL
		res.Title = info.Title
	}
	return res, nil
}
