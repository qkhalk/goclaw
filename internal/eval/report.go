package eval

import (
	"fmt"
	"io"
	"strings"
)

// WriteReport prints the eval score table: one PASS/FAIL line per case and a
// per-suite score summary. Mirrors the layered-test conventions (P0 failures
// are blocking). Returns true when every case passed.
func WriteReport(w io.Writer, reports []SuiteReport, onlyFailures bool) bool {
	allPass := true
	var totalCaseLines int
	for _, r := range reports {
		fmt.Fprintf(w, "\n== %s (driver: %s) ==\n", r.Suite, r.Driver)
		for _, res := range r.Results {
			if res.Passed && onlyFailures {
				continue
			}
			status := "PASS"
			if !res.Passed {
				status = "FAIL"
				allPass = false
			}
			fmt.Fprintf(w, "  %-4s %s  %s (%s, %s)\n", status, res.Severity, res.Name, res.Duration.Round(roundMs), res.Suite)
			if !res.Passed {
				totalCaseLines++
				indent := strings.Repeat(" ", 11)
			if res.Err != "" {
				fmt.Fprintf(w, "%s%s\n", indent, wordWrap(res.Err, 100, indent))
			}
			if res.Detail != "" {
				fmt.Fprintf(w, "%sretrieved:\n", indent)
				for _, line := range strings.Split(res.Detail, "\n") {
					fmt.Fprintf(w, "%s  %s\n", indent, line)
				}
			}
			}
		}
	}

	fmt.Fprintln(w, "\n== Score ==")
	for _, r := range reports {
		status := "OK"
		if r.Failed > 0 {
			status = "FAIL"
		}
		fmt.Fprintf(w, "  %-28s %3d%%  (%d/%d passed)  %s  [%s]\n",
			r.Suite, r.Score(), r.Passed, r.Total, r.Duration.Round(roundMs), status)
	}
	return allPass
}

const roundMs = 1e6 // round durations to milliseconds

// SummaryLine is the one-line verdict for CLI exit handling.
func SummaryLine(reports []SuiteReport) string {
	var total, passed, suitesFailed int
	for _, r := range reports {
		total += r.Total
		passed += r.Passed
		if r.Failed > 0 {
			suitesFailed++
		}
	}
	verdict := "PASS"
	if suitesFailed > 0 {
		verdict = "FAIL"
	}
	return fmt.Sprintf("%s: %d/%d cases passed, %d/%d suites failed",
		verdict, passed, total, suitesFailed, len(reports))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

// wordWrap reflows msg to width chars with continuation lines indented by
// indent, keeping failure output readable in narrow terminals.
func wordWrap(msg string, width int, indent string) string {
	if width <= 0 || len(msg) <= width {
		return msg
	}
	var out []string
	for len(msg) > width {
		cut := strings.LastIndexByte(msg[:width], ' ')
		if cut <= 0 {
			cut = width
		}
		out = append(out, strings.TrimRight(msg[:cut], " "))
		msg = strings.TrimLeft(msg[cut:], " ")
	}
	out = append(out, msg)
	return strings.Join(out, "\n"+indent)
}
