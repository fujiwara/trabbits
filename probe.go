package trabbits

import "time"

type probeLog struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	attrs     []any     // Store as slice to avoid allocation when not consumed
}

// AttrsMap converts the internal attrs slice to a map when needed (e.g., for JSON marshaling)
func (p *probeLog) AttrsMap() map[string]any {
	if len(p.attrs) == 0 {
		return nil
	}

	m := make(map[string]any)
	for i := 0; i < len(p.attrs)-1; i += 2 {
		if key, ok := p.attrs[i].(string); ok {
			m[key] = p.attrs[i+1]
		}
	}
	return m
}
