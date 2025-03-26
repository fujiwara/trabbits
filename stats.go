package trabbits

import "time"

type ProxyStats struct {
	ConnectedAt  time.Time `json:"connected_at"`
	ClosedAt     time.Time `json:"closed_at"`
	ProxyID      string    `json:"proxy_id"`
	ClientAddr   string    `json:"client_addr"`
	ClientBanner string    `json:"client_banner"`
	Upstreams    []string  `json:"upstreams"`
}

func (s *Proxy) updateStats(modify func(st *ProxyStats)) {
	key := s.ClientAddr()
	if st, ok := proxyStats.Load(key); ok {
		st := st.(*ProxyStats)
		if st != nil {
			modify(st)
		}
		proxyStats.Store(key, st)
		return
	} else {
		st := &ProxyStats{
			ProxyID:    s.id,
			ClientAddr: key,
		}
		if modify != nil {
			modify(st)
		}
		proxyStats.Store(key, st)
	}
}
