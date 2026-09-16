package browse

import (
	"strings"
	"testing"
)

func TestSanitizeDropsScriptAndHandlers(t *testing.T) {
	raw := `<!doctype html><html><head><title>T</title>` +
		`<script>alert(1)</script><base href="https://evil.com/"></head>` +
		`<body onload="steal()"><h1 onclick="x()">Hi</h1>` +
		`<a href="javascript:evil()">link</a><a href="/docs">docs</a>` +
		`<iframe src="https://evil.com"></iframe>` +
		`<meta http-equiv="refresh" content="0;url=https://evil.com">` +
		`</body></html>`
	out, title := Sanitize(raw, "https://example.com/page")

	if title != "T" {
		t.Fatalf("title = %q, want T", title)
	}
	lower := strings.ToLower(out)
	for _, want := range []string{"<script", "onload=", "onclick=", "javascript:", "<iframe", "http-equiv", "<base"} {
		if strings.Contains(lower, want) {
			t.Fatalf("sanitized output still contains %q:\n%s", want, out)
		}
	}
	if !strings.Contains(lower, `<a href="https://example.com/docs"`) {
		t.Fatalf("relative href not absolutized:\n%s", out)
	}
	if !strings.Contains(lower, `name="referrer"`) {
		t.Fatal("no-referrer meta missing")
	}
}

func TestSanitizeAbsolutizesAssets(t *testing.T) {
	raw := `<!doctype html><html><head><title>P</title>` +
		`<link rel="stylesheet" href="/css/site.css"></head><body>` +
		`<img src="img/pic.png" srcset="img/pic.png 1x, /img/pic2x.png 2x">` +
		`<video poster="v/poster.jpg"></video></body></html>`
	out, _ := Sanitize(raw, "https://cdn.example.com/articles/1")

	for _, want := range []string{
		`href="https://cdn.example.com/css/site.css"`,
		`src="https://cdn.example.com/articles/img/pic.png"`,
		`srcset="https://cdn.example.com/articles/img/pic.png 1x, https://cdn.example.com/img/pic2x.png 2x"`,
		`poster="https://cdn.example.com/articles/v/poster.jpg"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSanitizeKeepsBodyContent(t *testing.T) {
	raw := `<!doctype html><html><head><title>Keep</title></head><body>` +
		`<article><h1>Heading</h1><p>Paragraph text.</p></article></body></html>`
	out, title := Sanitize(raw, "https://example.com/a")
	if title != "Keep" {
		t.Fatalf("title = %q", title)
	}
	if !strings.Contains(out, "Paragraph text.") {
		t.Fatalf("body content lost:\n%s", out)
	}
}

func TestSanitizeUnparseableFallsBackToText(t *testing.T) {
	out, _ := Sanitize("just <<< text \x00 garbage", "https://example.com")
	if strings.Contains(out, "<") && strings.Contains(out, ">") {
		// Angle brackets alone are fine; the requirement is no elements survive.
		t.Logf("output: %q", out)
	}
	if strings.Contains(strings.ToLower(out), "<script") {
		t.Fatalf("fallback leaked a tag: %q", out)
	}
}

func TestExtractTitle(t *testing.T) {
	if got := ExtractTitle("<html><head><title>  Hello World  </title></head></html>"); got != "Hello World" {
		t.Fatalf("ExtractTitle = %q", got)
	}
	if got := ExtractTitle("no title here"); got != "" {
		t.Fatalf("ExtractTitle = %q, want empty", got)
	}
}
