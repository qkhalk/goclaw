package browse

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Elements dropped entirely (element + subtree). Scripts must never execute
// in the relayed document: it is served same-origin, so page JS would run
// with access to the dashboard origin. Frames/objects/embeds are nested
// browsing contexts we cannot sanitize recursively.
var dropElements = map[atom.Atom]bool{
	atom.Script:   true,
	atom.Noscript: true,
	atom.Iframe:   true,
	atom.Frame:    true,
	atom.Frameset: true,
	atom.Object:   true,
	atom.Embed:    true,
	atom.Applet:   true,
	atom.Base:     true, // we resolve URLs ourselves; a foreign base would misroute them
}

// Attributes carrying URLs that must be absolutized (so assets load from the
// origin site directly on the client) or dropped when scripted.
var urlAttrs = map[string]bool{
	"href":       true,
	"src":        true,
	"poster":     true,
	"action":     true,
	"formaction": true,
	"data":       true,
}

// Sanitize rewrites a fetched HTML document for same-origin relay into the
// web client's browser panel:
//
//   - drops script-capable elements (script/noscript/frames/objects/base);
//   - strips every on* event-handler attribute and javascript:/vbscript:/
//     data:text/html URLs;
//   - resolves relative URLs against the final (post-redirect) page URL so
//     every subresource loads from the origin site on the CLIENT, not via
//     the server;
//   - drops <meta http-equiv=refresh> (auto-navigation) and injects
//     <meta name="referrer" content="no-referrer">.
//
// It returns the sanitized document and the page <title>. The output is
// defense-in-depth: the relay response also carries a script-src 'none' CSP
// and the client renders it inside a sandboxed iframe.
func Sanitize(rawHTML, baseURL string) (string, string) {
	base, err := url.Parse(baseURL)
	if err != nil {
		base = nil
	}

	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		// Unparseable input: strip everything tag-like rather than relay it.
		return stripAllTags(rawHTML), ""
	}

	title := findTitle(doc)
	sanitizeNode(doc, base)
	injectNoReferrer(doc)

	var sb strings.Builder
	if err := html.Render(&sb, doc); err != nil {
		return stripAllTags(rawHTML), title
	}
	return sb.String(), title
}

// sanitizeNode walks the tree in place, dropping unsafe elements and
// rewriting attributes. Returns the node that should follow n in the parent's
// child list after any removals.
func sanitizeNode(n *html.Node, base *url.URL) {
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode && dropElements[child.DataAtom] {
			// Meta refresh is a <meta>, not in dropElements — handled below.
			n.RemoveChild(child)
		} else {
			if child.Type == html.ElementNode {
				if child.DataAtom == atom.Meta && isMetaRefresh(child) {
					n.RemoveChild(child)
					child = next
					continue
				}
				sanitizeAttrs(child, base)
			}
			sanitizeNode(child, base)
		}
		child = next
	}
}

// sanitizeAttrs strips event handlers and scripted URLs, and absolutizes
// remaining URL attributes against the page's final URL.
func sanitizeAttrs(n *html.Node, base *url.URL) {
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if strings.HasPrefix(key, "on") {
			continue
		}
		if key == "srcset" {
			if base != nil {
				a.Val = absolutizeSrcset(a.Val, base)
			} else {
				continue
			}
			kept = append(kept, a)
			continue
		}
		if urlAttrs[key] {
			if isScriptedURL(a.Val) {
				continue
			}
			if base != nil && a.Val != "" {
				if resolved, err := base.Parse(strings.TrimSpace(a.Val)); err == nil {
					a.Val = resolved.String()
				} else {
					continue
				}
			}
		}
		kept = append(kept, a)
	}
	n.Attr = kept
}

// isScriptedURL reports whether a URL value uses an executable scheme.
func isScriptedURL(val string) bool {
	v := strings.TrimSpace(strings.ToLower(val))
	return strings.HasPrefix(v, "javascript:") ||
		strings.HasPrefix(v, "vbscript:") ||
		strings.HasPrefix(v, "data:text/html")
}

// absolutizeSrcset resolves every candidate URL in a srcset value.
func absolutizeSrcset(val string, base *url.URL) string {
	candidates := strings.Split(val, ",")
	for i, cand := range candidates {
		parts := strings.Fields(strings.TrimSpace(cand))
		if len(parts) == 0 {
			continue
		}
		if resolved, err := base.Parse(parts[0]); err == nil {
			parts[0] = resolved.String()
			candidates[i] = strings.Join(parts, " ")
		}
	}
	return strings.Join(candidates, ", ")
}

// isMetaRefresh detects <meta http-equiv="refresh" ...>.
func isMetaRefresh(n *html.Node) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "http-equiv") && strings.EqualFold(strings.TrimSpace(a.Val), "refresh") {
			return true
		}
	}
	return false
}

// injectNoReferrer prepends <meta name="referrer" content="no-referrer"> to
// the document head so relayed subresource requests leak no referrer.
func injectNoReferrer(doc *html.Node) {
	head := findFirst(doc, atom.Head)
	if head == nil {
		return
	}
	meta := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Meta,
		Data:     "meta",
		Attr: []html.Attribute{
			{Key: "name", Val: "referrer"},
			{Key: "content", Val: "no-referrer"},
		},
	}
	head.InsertBefore(meta, head.FirstChild)
}

func findFirst(n *html.Node, want atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == want {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, want); found != nil {
			return found
		}
	}
	return nil
}

func findTitle(doc *html.Node) string {
	t := findFirst(doc, atom.Title)
	if t == nil {
		return ""
	}
	var sb strings.Builder
	collectText(t, &sb)
	return strings.TrimSpace(sb.String())
}

// ExtractTitle returns the page <title> without any other rewriting. Used by
// the web_browse fallback path, which never relays the document.
func ExtractTitle(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return ""
	}
	return findTitle(doc)
}

func collectText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, sb)
	}
}

// stripAllTags is the unparseable-input fallback: keep only text content.
func stripAllTags(raw string) string {
	var sb strings.Builder
	z := html.NewTokenizer(strings.NewReader(raw))
	for {
		tt := z.Next()
		switch tt {
		case html.TextToken:
			sb.Write(z.Text())
		case html.ErrorToken:
			return sb.String()
		}
	}
}
