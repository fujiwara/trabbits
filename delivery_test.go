package trabbits_test

import (
	"testing"

	"github.com/fujiwara/trabbits"
)

func TestDelivery(t *testing.T) {
	t.Parallel()
	n := 2
	for _tag := range 10 {
		tag := uint64(_tag)
		for i := range 1 {
			d := trabbits.NewDelivery(nil, i, n)
			clientDeliveryTag := d.Tag(tag)
			rt, ri := trabbits.RestoreDeliveryTag(clientDeliveryTag, n)
			if rt != tag || ri != i {
				t.Errorf("tag mismatch: %d", tag)
			}
		}
	}
}
