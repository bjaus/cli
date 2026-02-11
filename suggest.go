package cli

import (
	"fmt"
	"math"
	"strings"
)

const suggestionThreshold = 0.7

// jaroWinkler computes the Jaro-Winkler similarity between two strings.
// Returns a value between 0.0 (no similarity) and 1.0 (identical).
func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	matchDist := int(math.Max(float64(len(s1)), float64(len(s2))))/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	s1Matches := make([]bool, len(s1))
	s2Matches := make([]bool, len(s2))

	var matches, transpositions float64

	for i := range len(s1) {
		start := max(0, i-matchDist)
		end := min(len(s2), i+matchDist+1)

		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := range len(s1) {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	jaro := (matches/float64(len(s1)) + matches/float64(len(s2)) +
		(matches-transpositions/2)/matches) / 3

	// Winkler modification: boost for common prefix (up to 4 chars)
	prefixLen := 0
	for i := range min(len(s1), len(s2), 4) {
		if s1[i] != s2[i] {
			break
		}
		prefixLen++
	}

	return jaro + float64(prefixLen)*0.1*(1-jaro)
}

// suggestSubcommand finds the closest matching subcommand name.
func suggestSubcommand(cmd Commander, unknown string) string {
	subs, _ := allSubcommands(cmd) //nolint:errcheck // best-effort suggestion
	if len(subs) == 0 {
		return ""
	}

	var bestName string
	var bestScore float64

	for _, sub := range subs {
		info := resolveInfo(sub)
		if info.hidden {
			continue
		}

		score := jaroWinkler(unknown, info.name)
		if score > bestScore {
			bestScore = score
			bestName = info.name
		}

		for _, alias := range info.aliases {
			score := jaroWinkler(unknown, alias)
			if score > bestScore {
				bestScore = score
				bestName = alias
			}
		}
	}

	if bestScore >= suggestionThreshold {
		return bestName
	}
	return ""
}

// suggestFlagName finds the closest matching flag name.
func suggestFlagName(cmd Commander, unknown string) string {
	flags := ScanFlags(cmd)
	if len(flags) == 0 {
		return ""
	}

	// Strip leading dashes for comparison
	stripped := strings.TrimLeft(unknown, "-")

	var bestName string
	var bestScore float64

	for i := range flags {
		f := &flags[i]
		score := jaroWinkler(stripped, f.Name)
		if score > bestScore {
			bestScore = score
			bestName = "--" + f.Name
		}
		if f.Short != "" {
			score := jaroWinkler(stripped, f.Short)
			if score > bestScore {
				bestScore = score
				bestName = "-" + f.Short
			}
		}
	}

	if bestScore >= suggestionThreshold {
		return bestName
	}
	return ""
}

// suggestFromError extracts the unknown token from a parse error and suggests.
func suggestFromError(cmd Commander, err error) string {
	msg := err.Error()

	if strings.HasPrefix(msg, "unknown flag: ") {
		unknown := strings.TrimPrefix(msg, "unknown flag: ")
		if suggestion := suggestFlagName(cmd, unknown); suggestion != "" {
			return fmt.Sprintf("Did you mean %q?", suggestion)
		}
	}

	return ""
}
