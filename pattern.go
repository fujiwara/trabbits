// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"strings"
)

func matchPattern(routingKey, bindingPattern string) bool {
	// Special case - identical strings always match
	if routingKey == bindingPattern {
		return true
	}

	// If we have special characters in the pattern
	if strings.Contains(bindingPattern, "*") || strings.Contains(bindingPattern, "#") {
		// Split by dots
		routingParts := strings.Split(routingKey, ".")
		patternParts := strings.Split(bindingPattern, ".")

		// If the pattern contains a dot, the routing key must also contain at least one dot
		if strings.Contains(bindingPattern, ".") && !strings.Contains(routingKey, ".") {
			return false
		}

		// If different number of parts and no # wildcard, no match
		if len(routingParts) != len(patternParts) && !strings.Contains(bindingPattern, "#") {
			return false
		}

		return matchParts(routingParts, patternParts)
	}

	// No wildcards, must be exact match
	return routingKey == bindingPattern
}

// matchParts matches the tokenized parts of the routing key and pattern
func matchParts(routingParts, patternParts []string) bool {
	// Process each part
	routingIndex := 0
	patternIndex := 0

	for routingIndex < len(routingParts) && patternIndex < len(patternParts) {
		patternPart := patternParts[patternIndex]

		// Handle # wildcard (matches 0 or more words)
		if patternPart == "#" {
			patternIndex++

			// # at the end matches everything remaining
			if patternIndex == len(patternParts) {
				return true
			}

			// Find next matching part after #
			for ; routingIndex < len(routingParts); routingIndex++ {
				// Try to match the rest of the pattern starting from current routing part
				if matchParts(routingParts[routingIndex:], patternParts[patternIndex:]) {
					return true
				}
			}

			// If we get here, no match was found
			return false
		}

		// Handle * wildcard or parts with embedded wildcards
		if patternPart == "*" || strings.Contains(patternPart, "*") || strings.Contains(patternPart, "#") {
			if !matchPartWithWildcards(routingParts[routingIndex], patternPart) {
				return false
			}
		} else if patternPart != routingParts[routingIndex] {
			// Literal parts must match exactly
			return false
		}

		routingIndex++
		patternIndex++
	}

	// Check if we've consumed all parts
	return routingIndex == len(routingParts) && patternIndex == len(patternParts)
}

// matchPartWithWildcards matches a single routing part against a pattern part that might contain wildcards
func matchPartWithWildcards(routingPart, patternPart string) bool {
	// If pattern is just "*", it matches any single word
	if patternPart == "*" {
		return true
	}

	// Handle embedded wildcards (e.g., "foo-*", "bar-#")
	if strings.Contains(patternPart, "*") || strings.Contains(patternPart, "#") {
		// Convert to regex-like pattern for matching
		// For simplicity, we'll do a basic implementation

		// Replace * with ".*" and # with ".*"
		patternRegex := strings.ReplaceAll(patternPart, "*", ".*")
		patternRegex = strings.ReplaceAll(patternRegex, "#", ".*")

		// Split the pattern at the wildcard to get prefix and suffix
		var prefix, suffix string

		if strings.Contains(patternPart, "*") {
			parts := strings.Split(patternPart, "*")
			if len(parts) > 0 {
				prefix = parts[0]
			}
			if len(parts) > 1 {
				suffix = parts[len(parts)-1]
			}
		} else if strings.Contains(patternPart, "#") {
			parts := strings.Split(patternPart, "#")
			if len(parts) > 0 {
				prefix = parts[0]
			}
			if len(parts) > 1 {
				suffix = parts[len(parts)-1]
			}
		}

		// Check if routing part starts with prefix and ends with suffix
		return (prefix == "" || strings.HasPrefix(routingPart, prefix)) &&
			(suffix == "" || strings.HasSuffix(routingPart, suffix))
	}

	// No wildcards, must be exact match
	return routingPart == patternPart
}
