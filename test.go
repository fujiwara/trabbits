// MIT License
// Copyright (c) 2025 FUJIWARA Shunichiro

package trabbits

import (
	"context"
	"fmt"

	"github.com/fujiwara/trabbits/pattern"
)

func testMatchRouting(_ context.Context, cli *CLI) error {
	bindingPattern := cli.Test.MatchRouting.Pattern
	key := cli.Test.MatchRouting.Key

	result := pattern.Match(key, bindingPattern)

	if result {
		fmt.Printf("✓ MATCHED: pattern '%s' matches key '%s'\n", bindingPattern, key)
	} else {
		fmt.Printf("✗ NOT MATCHED: pattern '%s' does not match key '%s'\n", bindingPattern, key)
	}

	if !result {
		return fmt.Errorf("pattern did not match")
	}
	return nil
}
