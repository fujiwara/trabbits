// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"
)

func testMatchRouting(_ context.Context, cli *CLI) error {
	pattern := cli.Test.MatchRouting.Pattern
	key := cli.Test.MatchRouting.Key

	result := matchPattern(key, pattern)

	if result {
		fmt.Printf("✓ MATCHED: pattern '%s' matches key '%s'\n", pattern, key)
	} else {
		fmt.Printf("✗ NOT MATCHED: pattern '%s' does not match key '%s'\n", pattern, key)
	}

	if !result {
		return fmt.Errorf("pattern did not match")
	}
	return nil
}
