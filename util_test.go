package trabbits_test

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func dumpMetrics() {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		panic(err)
		return
	}

	enc := expfmt.NewEncoder(os.Stdout, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		enc.Encode(mf)
	}
}
