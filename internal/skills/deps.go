package skills

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Skill dependency resolution.
//
// SKILL.md frontmatter may declare inter-skill relationships via three
// top-level keys (block-list or comma-separated scalar form):
//
//	---
//	depends:
//	  - review-pr
//	  - plan@>=1.2
//	requires:
//	  - cook@==2.0.0
//	conflicts:
//	  - legacy-cook
//	---
//
// `depends` and `requires` are aliases feeding the same dependency list;
// `conflicts` declares mutually exclusive skills. Dependency references are
// either a bare slug or `slug@CONSTRAINT` where CONSTRAINT is `==X.Y.Z`
// (exact) or `>=X.Y` (minimum), compared against the dependency's declared
// frontmatter `version`.
//
// Resolve walks the transitive closure of a requested slug set and produces a
// deterministic install plan: topologically ordered installs (dependencies
// first), already-present skills, unsatisfiable requirements, conflicts
// within the closure, and dependency cycles.

// ErrSkillNotFound is returned by a Resolve lookup callback when the slug is
// unknown to the resolution universe. It records a Missing entry rather than
// aborting resolution.
var ErrSkillNotFound = errors.New("skills: skill not found")

// ErrSkillInstalled is returned by a Resolve lookup callback when the slug is
// already present in the destination and must not be reinstalled. The
// accompanying content and version still describe the skill so its own
// dependencies and constraints continue to resolve; the slug lands in
// Plan.AlreadySatisfied instead of Plan.Install.
var ErrSkillInstalled = errors.New("skills: skill already installed")

// ParseDepends extracts dependency and conflict declarations from raw
// SKILL.md content. Dependencies come from the `depends` and `requires`
// frontmatter keys (concatenated, declaration order preserved, duplicates
// removed); conflicts come from `conflicts`. Both block-list and
// comma-separated scalar forms are accepted.
//
// Every dependency reference must be a bare slug or `slug@CONSTRAINT` with a
// `==`/`>=` operator and a dotted numeric version; conflict entries must be
// bare slugs. Malformed entries yield an error naming the offender.
func ParseDepends(content string) (deps []string, conflicts []string, err error) {
	fm := extractFrontmatter(content)
	if fm == "" {
		return nil, nil, nil
	}

	lists := parseSimpleYAMLLists(fm)
	kv := parseSimpleYAML(fm)

	deps, err = mergeDeclaredKeys(lists, kv, "depends", "requires")
	if err != nil {
		return nil, nil, err
	}
	conflicts, err = mergeDeclaredKeys(lists, kv, "conflicts")
	if err != nil {
		return nil, nil, err
	}
	return deps, conflicts, nil
}

// mergeDeclaredKeys collects validated entries from the given frontmatter
// keys, preserving first-seen order and dropping duplicates.
func mergeDeclaredKeys(lists map[string][]string, kv map[string]string, keys ...string) ([]string, error) {
	var (
		seen  = map[string]bool{}
		order []string
	)
	add := func(raw string) error {
		entry, err := validateReference(raw)
		if err != nil {
			return err
		}
		if !seen[entry] {
			seen[entry] = true
			order = append(order, entry)
		}
		return nil
	}
	for _, key := range keys {
		for _, item := range lists[key] {
			if err := add(item); err != nil {
				return nil, err
			}
		}
		if len(lists[key]) == 0 && kv[key] != "" {
			for _, item := range splitListValue(kv[key]) {
				if err := add(item); err != nil {
					return nil, err
				}
			}
		}
	}
	return order, nil
}

// validateReference normalizes a dependency reference (`slug` or
// `slug@CONSTRAINT`) and enforces the reference grammar.
func validateReference(raw string) (string, error) {
	ref := strings.TrimSpace(strings.Trim(raw, "\"'"))
	if ref == "" {
		return "", fmt.Errorf("skills: empty dependency reference")
	}
	slug := ref
	constraint := ""
	if i := strings.LastIndexByte(ref, '@'); i >= 0 {
		slug, constraint = ref[:i], ref[i+1:]
		if constraint == "" {
			return "", fmt.Errorf("skills: dangling version separator in dependency %q", raw)
		}
		if _, version, ok := splitConstraint(constraint); !ok || !isNumericVersion(version) {
			return "", fmt.Errorf("skills: invalid version constraint %q in %q (want \"==X.Y.Z\" or \">=X.Y\")", constraint, raw)
		}
	}
	// Reuse the package's canonical slug grammar so dependency references
	// can never name something the managed layout could not store.
	if !SlugRegexp.MatchString(slug) {
		return "", fmt.Errorf("skills: invalid dependency slug %q", raw)
	}
	return ref, nil
}

// splitConstraint splits an operator-prefixed constraint into its operator
// and version. Only `==` and `>=` are recognized.
func splitConstraint(constraint string) (op, version string, ok bool) {
	switch {
	case strings.HasPrefix(constraint, "=="):
		return "==", strings.TrimPrefix(constraint, "=="), true
	case strings.HasPrefix(constraint, ">="):
		return ">=", strings.TrimPrefix(constraint, ">="), true
	default:
		return "", constraint, false
	}
}

// isNumericVersion reports whether version is a dotted numeric version such
// as `1.2` or `1.2.3`. Pre-release suffixes are rejected so comparisons are
// never silently approximate.
func isNumericVersion(version string) bool {
	if version == "" {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// versionParts converts a version string into numeric components. Each
// dot-separated component contributes its leading integer digits; anything
// non-numeric parses as zero, so "1.2.0-beta" compares equal to "1.2.0".
func versionParts(version string) []int {
	parts := strings.Split(strings.TrimSpace(version), ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n := 0
		i := 0
		for i < len(part) && part[i] >= '0' && part[i] <= '9' {
			n = n*10 + int(part[i]-'0')
			i++
		}
		out = append(out, n)
	}
	return out
}

// compareVersions numerically compares two dotted versions, padding the
// shorter with zeroes: returns -1 when a < b, 0 when a == b, 1 when a > b.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(pa) {
			av = pa[i]
		}
		if i < len(pb) {
			bv = pb[i]
		}
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
	}
	return 0
}

// versionSatisfies reports whether version fulfills constraint ("==X.Y.Z" or
// ">=X.Y"). An empty constraint accepts everything.
func versionSatisfies(version, constraint string) bool {
	if constraint == "" {
		return true
	}
	op, want, ok := splitConstraint(constraint)
	if !ok {
		return false
	}
	cmp := compareVersions(version, want)
	switch op {
	case "==":
		return cmp == 0
	case ">=":
		return cmp >= 0
	default:
		return false
	}
}

// MissingDep describes a slug that cannot be satisfied: either unknown to
// the resolution universe or pinned by a version constraint no available
// version fulfills. RequiredBy lists the direct requirers (empty when the
// slug itself was requested); downstream prunes of dependents are implied.
type MissingDep struct {
	Slug       string
	RequiredBy []string
}

// Plan is the outcome of resolving a requested slug set against a lookup
// universe.
//
//   - Install: slugs to materialize, topologically ordered (dependencies
//     first), deduplicated. Members whose subtree contains any unsatisfiable
//     requirement or dependency cycle are excluded.
//   - AlreadySatisfied: closure members reported installed by the lookup;
//     sorted. Their dependencies still participate in resolution.
//   - Missing: unsatisfiable slugs, sorted by slug.
//   - Conflicts: slugs declared conflicting by some closure member while also
//     present in the closure; sorted, deduplicated, self-references ignored.
//     Install is left complete for inspection; callers must refuse to act
//     when Conflicts is non-empty.
//   - Cycles: dependency cycles, each rotated to start at its
//     lexicographically smallest slug; sorted.
type Plan struct {
	Install          []string
	AlreadySatisfied []string
	Missing          []MissingDep
	Conflicts        []string
	Cycles           [][]string
}

// depNode is a resolved skill snapshot within the closure.
type depNode struct {
	version   string
	installed bool // reported by ErrSkillInstalled; excluded from Install
	deps      []dependencyRef
	ok        bool // subtree fully resolvable
	emitted   bool
}

// dependencyRef is a parsed `slug@constraint` edge.
type dependencyRef struct {
	slug       string
	constraint string
}

// DFS node colors: white is the zero value (unvisited), gray marks the stack,
// black marks resolved subtrees.
const (
	stateWhite = iota
	stateGray
	stateBlack
)

// Resolve computes the transitive dependency closure of slugs via lookup,
// which must return the SKILL.md content and declared frontmatter version for
// a slug, or ErrSkillNotFound / ErrSkillInstalled as described above. Any
// other lookup error aborts resolution.
//
// Resolution is deterministic: dependencies are traversed in declaration
// order and roots in request order, so Install order is stable across runs.
func Resolve(slugs []string, lookup func(slug string) (content string, version string, err error)) (Plan, error) {
	plan := Plan{}

	// Deduplicate roots, preserving request order.
	var roots []string
	rootSeen := map[string]bool{}
	for _, s := range slugs {
		s = strings.TrimSpace(s)
		if s == "" || rootSeen[s] {
			continue
		}
		rootSeen[s] = true
		roots = append(roots, s)
	}

	nodes := map[string]*depNode{}
	status := map[string]int{} // slug → stateWhite/stateGray/stateBlack
	missing := map[string]*MissingDep{}
	cycleMembers := map[string]bool{}
	var cycles [][]string
	cycleSeen := map[string]bool{}
	declaredConflicts := map[string][]string{} // declarer → targets
	var visited []string                       // every slug that entered the DFS, discovery order
	inClosure := map[string]bool{}

	recordMissing := func(slug, requiredBy string) {
		m, ok := missing[slug]
		if !ok {
			m = &MissingDep{Slug: slug}
			missing[slug] = m
		}
		if requiredBy == "" {
			return
		}
		for _, r := range m.RequiredBy {
			if r == requiredBy {
				return
			}
		}
		m.RequiredBy = append(m.RequiredBy, requiredBy)
	}

	// normalizeCycle rotates the cycle slice so it starts at its
	// lexicographically smallest member, giving every rotation of the same
	// cycle one canonical form.
	var normalizeCycle func(cycle []string) []string
	normalizeCycle = func(cycle []string) []string {
		min := 0
		for i, s := range cycle {
			if s < cycle[min] {
				min = i
			}
		}
		out := make([]string, 0, len(cycle))
		out = append(out, cycle[min:]...)
		out = append(out, cycle[:min]...)
		return out
	}

	var visit func(slug string, path []string) (bool, error)
	visit = func(slug string, path []string) (bool, error) {
		switch status[slug] {
		case stateBlack:
			return nodes[slug].ok, nil
		case stateGray:
			// Cycle: extract the stack slice starting at the previous
			// occurrence of slug.
			start := 0
			for i, p := range path {
				if p == slug {
					start = i
					break
				}
			}
			cycle := normalizeCycle(append([]string{}, path[start:]...))
			key := strings.Join(cycle, "\x00")
			if !cycleSeen[key] {
				cycleSeen[key] = true
				cycles = append(cycles, cycle)
			}
			for _, member := range cycle {
				cycleMembers[member] = true
			}
			return false, nil
		}

		content, version, err := lookup(slug)
		if errors.Is(err, ErrSkillNotFound) {
			// Left white so later requirers merge into RequiredBy.
			recordMissing(slug, requiredByOf(path))
			return false, nil
		}
		if err != nil && !errors.Is(err, ErrSkillInstalled) {
			return false, fmt.Errorf("skills: resolve %s: %w", slug, err)
		}
		installed := errors.Is(err, ErrSkillInstalled)

		depsRaw, conflictsRaw, perr := ParseDepends(content)
		if perr != nil {
			return false, fmt.Errorf("skills: resolve %s: %w", slug, perr)
		}
		declaredConflicts[slug] = conflictsRaw
		if !inClosure[slug] {
			inClosure[slug] = true
			visited = append(visited, slug)
		}

		deps := make([]dependencyRef, 0, len(depsRaw))
		for _, raw := range depsRaw {
			slugPart, constraint := splitDepRef(raw)
			deps = append(deps, dependencyRef{slug: slugPart, constraint: constraint})
		}
		nodes[slug] = &depNode{
			version:   version,
			installed: installed,
			deps:      deps,
		}

		status[slug] = stateGray
		childPath := append(path, slug)
		ok := true
		for _, ref := range deps {
			_, childVersion, cerr := lookup(ref.slug)
			childKnown := true
			switch {
			case errors.Is(cerr, ErrSkillNotFound):
				childKnown = false
			case errors.Is(cerr, ErrSkillInstalled):
				// Known; constraint checked against the installed version.
			case cerr != nil:
				return false, fmt.Errorf("skills: resolve %s: %w", ref.slug, cerr)
			}
			if childKnown && ref.constraint != "" && !versionSatisfies(childVersion, ref.constraint) {
				// Unsatisfiable pin: the requirement cannot be met by what
				// the universe offers, so treat it like not-found while the
				// edge stays recorded for RequiredBy attribution.
				childKnown = false
			}
			if !childKnown {
				recordMissing(ref.slug, slug)
				ok = false
				continue
			}
			edgeOK, err := visit(ref.slug, childPath)
			if err != nil {
				return false, err
			}
			if !edgeOK {
				ok = false
			}
		}
		status[slug] = stateBlack
		node := nodes[slug]
		node.ok = ok
		return ok, nil
	}

	for _, root := range roots {
		if _, err := visit(root, nil); err != nil {
			return Plan{}, err
		}
	}

	// Conflicts: a declared target that is itself inside the closure.
	sortedVisited := append([]string{}, visited...)
	sort.Strings(sortedVisited)
	for _, declarer := range sortedVisited {
		targets := append([]string{}, declaredConflicts[declarer]...)
		sort.Strings(targets)
		for _, target := range targets {
			if target == declarer || !inClosure[target] {
				continue
			}
			dup := false
			for _, c := range plan.Conflicts {
				if c == target {
					dup = true
					break
				}
			}
			if !dup {
				plan.Conflicts = append(plan.Conflicts, target)
			}
		}
	}

	// Emit in post-order (dependencies before dependents), skipping cycle
	// members and failed subtrees.
	var emit func(slug string)
	emit = func(slug string) {
		node, ok := nodes[slug]
		if !ok || node.emitted {
			return
		}
		node.emitted = true
		for _, ref := range node.deps {
			emit(ref.slug)
		}
		switch {
		case node.installed && !cycleMembers[slug]:
			plan.AlreadySatisfied = append(plan.AlreadySatisfied, slug)
		case !node.installed && node.ok && !cycleMembers[slug]:
			plan.Install = append(plan.Install, slug)
		}
	}
	for _, root := range roots {
		emit(root)
	}
	sort.Strings(plan.AlreadySatisfied)

	for _, slug := range sortedStrings(missing) {
		m := missing[slug]
		sort.Strings(m.RequiredBy)
		plan.Missing = append(plan.Missing, *m)
	}
	sort.Slice(cycles, func(i, j int) bool {
		return strings.Join(cycles[i], "\x00") < strings.Join(cycles[j], "\x00")
	})
	plan.Cycles = cycles

	return plan, nil
}

// requiredByOf returns the direct requirer of the node currently being
// resolved: the tail of the DFS path (empty for roots).
func requiredByOf(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

// splitDepRef splits a validated reference into slug and constraint.
func splitDepRef(ref string) (slug, constraint string) {
	if i := strings.LastIndexByte(ref, '@'); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}

// sortedStrings returns the keys of a string-keyed map in ascending order.
func sortedStrings[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
