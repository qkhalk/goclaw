package tools

import (
	"math"
	"strings"
	"unicode"
)

// SearchDoc is a single document indexed for BM25 tool search. It decouples
// the index from any concrete tool implementation so both the native deferred
// tools (internal/tools) and the MCP bridge tools (internal/mcp) share one
// ranking implementation.
type SearchDoc struct {
	Name        string // activation name (registry name for native, registeredName for MCP)
	Source      string // origin label: builtin tool group or MCP server name
	Title       string // underlying tool name (empty for native, original name for MCP)
	Description string // human-readable description used for matching
}

// SearchHit is a single scored result from a BM25 search.
type SearchHit struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description"`
	Score       float64 `json:"-"`
}

// bm25Doc is the internal indexed representation of a SearchDoc.
type bm25Doc struct {
	doc    SearchDoc
	tokens []string
}

// BM25Index is a minimal BM25 ranking index shared by tool-search meta-tools.
// It is not safe for concurrent mutation; callers rebuild it under their own
// synchronization (deferred sets are small, so a rebuild is cheap).
type BM25Index struct {
	docs  []bm25Doc
	df    map[string]int
	avgDL float64
	k1    float64
	b     float64
}

// NewBM25Index creates an empty index with the standard BM25 parameters.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		df: make(map[string]int),
		k1: 1.2,
		b:  0.75,
	}
}

// Build (re)indexes the given documents, discarding any previous contents.
func (idx *BM25Index) Build(docs []SearchDoc) {
	idx.docs = make([]bm25Doc, 0, len(docs))
	idx.df = make(map[string]int)

	totalTokens := 0

	for _, d := range docs {
		searchText := d.Source + " " + d.Title + " " + d.Name + " " + d.Description
		tokens := Tokenize(searchText)

		idx.docs = append(idx.docs, bm25Doc{doc: d, tokens: tokens})

		seen := make(map[string]bool)
		for _, t := range tokens {
			if !seen[t] {
				idx.df[t]++
				seen[t] = true
			}
		}

		totalTokens += len(tokens)
	}

	if len(idx.docs) > 0 {
		idx.avgDL = float64(totalTokens) / float64(len(idx.docs))
	}
}

// Search performs a BM25 search over the indexed documents, returning up to
// maxResults hits sorted by score descending. Ties preserve insertion order
// (stable), so identical input yields identical output.
func (idx *BM25Index) Search(query string, maxResults int) []SearchHit {
	if maxResults <= 0 {
		maxResults = 5
	}

	queryTokens := Tokenize(query)
	if len(queryTokens) == 0 || len(idx.docs) == 0 {
		return nil
	}

	N := float64(len(idx.docs))

	type scored struct {
		idx   int
		score float64
	}

	var results []scored

	for i, entry := range idx.docs {
		score := 0.0
		dl := float64(len(entry.tokens))

		tf := make(map[string]int)
		for _, t := range entry.tokens {
			tf[t]++
		}

		for _, qt := range queryTokens {
			termFreq := float64(tf[qt])
			if termFreq == 0 {
				continue
			}

			dfTerm := float64(idx.df[qt])
			idf := math.Log((N-dfTerm+0.5)/(dfTerm+0.5) + 1)

			numerator := termFreq * (idx.k1 + 1)
			denominator := termFreq + idx.k1*(1-idx.b+idx.b*dl/idx.avgDL)
			score += idf * numerator / denominator
		}

		if score > 0 {
			results = append(results, scored{idx: i, score: score})
		}
	}

	// Sort by score descending; insertion sort keeps ties in insertion
	// (index) order, making the output deterministic for equal scores.
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].score < key.score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]SearchHit, len(results))
	for i, r := range results {
		entry := idx.docs[r.idx]
		out[i] = SearchHit{
			Name:        entry.doc.Name,
			Source:      entry.doc.Source,
			Title:       entry.doc.Title,
			Description: entry.doc.Description,
			Score:       r.score,
		}
	}
	return out
}

// DocCount returns the number of indexed documents.
func (idx *BM25Index) DocCount() int { return len(idx.docs) }

// Tokenize splits text into lowercase word tokens, stripping punctuation and
// single-character tokens. Mirrors skills.tokenize() (unexported, so kept here).
func Tokenize(text string) []string {
	lower := strings.ToLower(text)

	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, lower)

	fields := strings.Fields(cleaned)

	var tokens []string
	for _, f := range fields {
		if len(f) > 1 {
			tokens = append(tokens, f)
		}
	}
	return tokens
}
