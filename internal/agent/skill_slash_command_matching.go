package agent

import (
	"sort"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

type parsedSkillSlashCommand struct {
	verb   string
	target string
	rest   string
}

func parseSkillSlashCommand(message, prefix string) (parsedSkillSlashCommand, bool) {
	message = strings.TrimSpace(message)
	if message == "" || !strings.HasPrefix(message, prefix) {
		return parsedSkillSlashCommand{}, false
	}
	after := strings.TrimSpace(strings.TrimPrefix(message, prefix))
	if after == "" || looksLikePath(after) {
		return parsedSkillSlashCommand{}, false
	}
	first, rest, _ := strings.Cut(after, " ")
	first = strings.TrimSpace(first)
	// Strip an @botname suffix from the command token: Telegram group menu
	// clicks send "/security_audit@BotName ..." and skill slugs never
	// contain "@" (mirrors handleBotCommand's suffix stripping).
	if at := strings.Index(first, "@"); at > 0 {
		first = first[:at]
	}
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(first) {
	case "list-skills":
		return parsedSkillSlashCommand{verb: "list-skills"}, true
	case "help":
		if rest == "" {
			return parsedSkillSlashCommand{}, false
		}
		return parsedSkillSlashCommand{verb: "help", target: rest}, true
	case "use", "activate":
		if rest == "" {
			return parsedSkillSlashCommand{}, false
		}
		return parsedSkillSlashCommand{verb: strings.ToLower(first), rest: rest}, true
	default:
		return parsedSkillSlashCommand{verb: "direct", target: first, rest: rest}, true
	}
}

func looksLikePath(value string) bool {
	first, _, _ := strings.Cut(value, " ")
	return strings.Contains(first, "/") || strings.Contains(first, "\\") || strings.Contains(first, ".")
}

func firstWord(value string) (string, string) {
	first, rest, ok := strings.Cut(strings.TrimSpace(value), " ")
	if !ok {
		return strings.TrimSpace(value), ""
	}
	return strings.TrimSpace(first), strings.TrimSpace(rest)
}

func matchSkillCommandTarget(all []skills.Info, raw string, partial bool) (skills.Info, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return skills.Info{}, false, ""
	}
	type candidate struct {
		info      skills.Info
		matchText string
		remainder string
		score     int
	}
	var matches []candidate
	lowerRaw := skillCommandNormalize(raw)
	partialTarget, partialRemainder := firstWord(raw)
	lowerPartialTarget := skillCommandNormalize(partialTarget)
	for _, skill := range all {
		for _, value := range []string{skill.Slug, skill.Name} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			lowerValue := skillCommandNormalize(value)
			if lowerRaw == lowerValue {
				matches = append(matches, candidate{info: skill, matchText: value, score: len([]rune(value))})
				continue
			}
			if strings.HasPrefix(lowerRaw, lowerValue+" ") {
				remainder := trimMatchedSkillCommandPrefix(raw, value)
				matches = append(matches, candidate{info: skill, matchText: value, remainder: remainder, score: len([]rune(value))})
				continue
			}
			if partial && lowerPartialTarget != "" && strings.HasPrefix(lowerValue, lowerPartialTarget) {
				matches = append(matches, candidate{info: skill, matchText: value, remainder: partialRemainder, score: len([]rune(partialTarget))})
			}
		}
	}
	if len(matches) == 0 {
		return skills.Info{}, false, ""
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	bestBySlug := make([]candidate, 0, len(matches))
	seenSlug := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if _, ok := seenSlug[match.info.Slug]; ok {
			continue
		}
		seenSlug[match.info.Slug] = struct{}{}
		bestBySlug = append(bestBySlug, match)
	}
	best := bestBySlug[0]
	if len(bestBySlug) > 1 && bestBySlug[0].score == bestBySlug[1].score {
		return skills.Info{}, false, ""
	}
	return best.info, true, best.remainder
}

// skillCommandNormalize lowercases and treats '_' as '-' so Telegram bot
// commands (which only allow [a-z0-9_], e.g. /security_audit) activate skills
// whose slugs use hyphens (security-audit). Two skills differing only by this
// normalization tie and therefore do not match (the equal-score rule above).
func skillCommandNormalize(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "_", "-")
}

func trimMatchedSkillCommandPrefix(raw, matched string) string {
	rawRunes := []rune(raw)
	matchedRunes := []rune(matched)
	if len(rawRunes) < len(matchedRunes) {
		return ""
	}
	return strings.TrimSpace(string(rawRunes[len(matchedRunes):]))
}
