// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"strings"
)

func matchPattern(routingKey, bindingPattern string) bool {
	// Split the routing key and binding pattern by dots
	routingTokens := strings.Split(routingKey, ".")
	bindingTokens := strings.Split(bindingPattern, ".")

	// Check for edge case: if pattern contains dots but routing key doesn't
	if len(bindingTokens) > 1 && len(routingTokens) == 1 && !strings.Contains(routingKey, ".") {
		return false // Pattern requires at least one dot, but routing key has none
	}

	return matchTokens(routingTokens, bindingTokens, 0, 0)
}

// matchTokens implements the recursive matching algorithm
func matchTokens(routingTokens, bindingTokens []string, rIdx, bIdx int) bool {
	// Base case: both patterns are exhausted
	if bIdx >= len(bindingTokens) {
		return rIdx >= len(routingTokens)
	}

	// If we've consumed all routing tokens but still have binding tokens
	if rIdx >= len(routingTokens) {
		// The only way to match is if all remaining binding tokens are #
		for i := bIdx; i < len(bindingTokens); i++ {
			if bindingTokens[i] != "#" {
				return false
			}
		}
		return true
	}

	// Current tokens
	rToken := routingTokens[rIdx]
	bToken := bindingTokens[bIdx]

	// Case 1: Current binding token is #
	if bToken == "#" {
		// Option 1: # matches zero tokens
		if matchTokens(routingTokens, bindingTokens, rIdx, bIdx+1) {
			return true
		}

		// Option 2: # matches the current token and potentially more
		return matchTokens(routingTokens, bindingTokens, rIdx+1, bIdx)
	}

	// Case 2: Current binding token is *
	if bToken == "*" {
		// * matches exactly one token
		return matchTokens(routingTokens, bindingTokens, rIdx+1, bIdx+1)
	}

	// Case 3: Check for % wildcard within the binding token
	if strings.Contains(bToken, "%") {
		// Use substring matching for this token
		if matchTokenWithPercent(rToken, bToken) {
			return matchTokens(routingTokens, bindingTokens, rIdx+1, bIdx+1)
		}
		return false
	}

	// Case 4: Literal match
	if rToken == bToken {
		return matchTokens(routingTokens, bindingTokens, rIdx+1, bIdx+1)
	}

	// No match
	return false
}

// matchTokenWithPercent handles % wildcards within a single token
// % matches any substring (including empty string) within the token
func matchTokenWithPercent(token, pattern string) bool {
	return matchPercentPattern(token, pattern, 0, 0)
}

// matchPercentPattern implements recursive pattern matching with % wildcard support
// This operates on a single token level, not across dot boundaries
func matchPercentPattern(text, pattern string, textIdx, patternIdx int) bool {
	// If we've reached the end of the pattern
	if patternIdx >= len(pattern) {
		return textIdx >= len(text) // Match if we've also consumed all text
	}

	// If we've reached the end of the text but pattern remains
	if textIdx >= len(text) {
		// Check if remaining pattern consists only of % wildcards
		for i := patternIdx; i < len(pattern); i++ {
			if pattern[i] != '%' {
				return false
			}
		}
		return true
	}

	// Handle % wildcard
	if pattern[patternIdx] == '%' {
		// Try matching % with empty string (skip the %)
		if matchPercentPattern(text, pattern, textIdx, patternIdx+1) {
			return true
		}

		// Try matching % with one or more characters
		return matchPercentPattern(text, pattern, textIdx+1, patternIdx)
	}

	// Literal character match (no * or # support within tokens)
	if pattern[patternIdx] == text[textIdx] {
		return matchPercentPattern(text, pattern, textIdx+1, patternIdx+1)
	}

	// No match
	return false
}
