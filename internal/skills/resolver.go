package skills

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// SkillResolver resolves SKILL.md files into SkillSpec objects, handling
// dependency resolution and validation. It wraps the existing Loader to
// maintain backward compatibility.
type SkillResolver struct {
	loader *Loader
	mu     sync.RWMutex
	// Cache of resolved specs by slug (invalidated on version bump)
	specCache map[string]*SkillSpec
	cacheVer  int64
}

// NewSkillResolver creates a resolver backed by the given Loader.
func NewSkillResolver(loader *Loader) *SkillResolver {
	return &SkillResolver{
		loader:    loader,
		specCache: make(map[string]*SkillSpec),
	}
}

// Resolve loads a skill by slug and returns its structured SkillSpec.
// Returns (nil, error) if the skill is not found or cannot be parsed.
func (r *SkillResolver) Resolve(ctx context.Context, slug string) (*SkillSpec, error) {
	r.mu.RLock()
	ver := r.loader.Version()
	if r.cacheVer == ver {
		if cached, ok := r.specCache[slug]; ok {
			r.mu.RUnlock()
			return cached, nil
		}
	}
	r.mu.RUnlock()

	// Load raw content
	info, ok := r.loader.GetSkill(ctx, slug)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", slug)
	}

	raw, err := readSkillFile(info.Path)
	if err != nil {
		return nil, fmt.Errorf("skill %q: %w", slug, err)
	}

	spec := specFromInfo(info, raw)

	// Parse dependency metadata from frontmatter
	spec.DependsOn = parseDependencyList(raw)
	spec.Provides = parseProvidesList(raw)

	// Parse execution hints
	spec.MaxDuration = parseMaxDuration(raw)
	spec.MaxRetries = parseMaxRetries(raw)

	r.mu.Lock()
	// Invalidate cache if version changed
	if r.cacheVer != ver {
		r.specCache = make(map[string]*SkillSpec)
		r.cacheVer = ver
	}
	r.specCache[slug] = spec
	r.mu.Unlock()

	return spec, nil
}

// ResolveAll loads all available skills and returns their specs.
func (r *SkillResolver) ResolveAll(ctx context.Context) ([]*SkillSpec, error) {
	infos := r.loader.ListSkills(ctx)
	var specs []*SkillSpec
	for _, info := range infos {
		spec, err := r.Resolve(ctx, info.Slug)
		if err != nil {
			continue // skip unresolvable skills
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// ResolveDependencies checks that all dependencies of a skill are available.
// Returns the ordered list of skills to load (dependencies first, then the skill itself).
// Returns error if a dependency is missing.
func (r *SkillResolver) ResolveDependencies(ctx context.Context, slug string) ([]*SkillSpec, error) {
	spec, err := r.Resolve(ctx, slug)
	if err != nil {
		return nil, err
	}

	visited := make(map[string]bool)
	var result []*SkillSpec

	var resolve func(s *SkillSpec) error
	resolve = func(s *SkillSpec) error {
		if visited[s.Slug] {
			return nil
		}
		visited[s.Slug] = true

		// Resolve dependencies first (depth-first)
		for _, dep := range s.DependsOn {
			depSpec, err := r.Resolve(ctx, dep)
			if err != nil {
				return fmt.Errorf("skill %q depends on %q: %w", slug, dep, err)
			}
			if err := resolve(depSpec); err != nil {
				return err
			}
		}

		result = append(result, s)
		return nil
	}

	if err := resolve(spec); err != nil {
		return nil, err
	}

	return result, nil
}

// ValidateSpec checks a SkillSpec for internal consistency.
func ValidateSpec(spec *SkillSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("skill spec: name is required")
	}
	if spec.Slug == "" {
		return fmt.Errorf("skill spec: slug is required")
	}
	if spec.MaxRetries < 0 {
		return fmt.Errorf("skill %q: maxRetries must be >= 0", spec.Slug)
	}
	if spec.MaxCost < 0 {
		return fmt.Errorf("skill %q: maxCost must be >= 0", spec.Slug)
	}
	return nil
}

// --- helpers ---

func specFromInfo(info *Info, raw string) *SkillSpec {
	return &SkillSpec{
		Name:         info.Name,
		Slug:         info.Slug,
		Description:  info.Description,
		Version:      info.Version,
		BaseDir:      info.BaseDir,
		Inputs:       info.Inputs,
		Outputs:      info.Outputs,
		AllowedTools: info.AllowedTools,
		QualityGates: info.QualityGates,
		Content:      stripFrontmatter(raw),
		Raw:          raw,
	}
}

// readSkillFile reads a file and returns its content.
func readSkillFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseDependencyList extracts "depends-on" from YAML frontmatter.
func parseDependencyList(raw string) []string {
	meta := parseSimpleYAML(raw)
	if v, ok := meta["depends-on"]; ok && v != "" {
		return splitListValue(v)
	}
	return nil
}

// parseProvidesList extracts "provides" from YAML frontmatter.
func parseProvidesList(raw string) []string {
	meta := parseSimpleYAML(raw)
	if v, ok := meta["provides"]; ok && v != "" {
		return splitListValue(v)
	}
	return nil
}

// parseMaxDuration extracts "max-duration" from YAML frontmatter.
func parseMaxDuration(raw string) time.Duration {
	meta := parseSimpleYAML(raw)
	if v, ok := meta["max-duration"]; ok && v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return 0
}

// parseMaxRetries extracts "max-retries" from YAML frontmatter.
func parseMaxRetries(raw string) int {
	meta := parseSimpleYAML(raw)
	if v, ok := meta["max-retries"]; ok && v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return 0 // default (will use executor default of 3)
}
