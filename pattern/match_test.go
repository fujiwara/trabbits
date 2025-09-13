package pattern

import (
	"fmt"
	"testing"
)

var matchTests = []struct {
	pattern string
	key     string
	want    bool
}{
	// literal case
	{"user.login", "user.login", true},
	{"user.login", "user.logout", false},
	{"user.profile.update", "user.profile.update", true},
	{"user.profile.update", "user.profile.delete", false},
	{"app.server.db", "app.server.db", true},
	{"app.server.db", "app.server", false},
	{"app.server", "app.server.db", false},
	{"foo", "foo", true},
	{"foo", "bar", false},

	// simple case
	{"user.*", "user.login", true},
	{"user.*", "user.profile.update", false},
	{"user.#", "user.profile.update", true},
	{"app.server.*", "app.server.db", true},
	{"app.server.*", "app.server.db.primary", false},
	{"app.#", "app.server.db.primary", true},

	// '#' pattern
	{"#", "random.text.here", true},
	{"#", "singleword", true},

	// '*' pattern
	{"system.*.log", "system.error.log", true},
	{"system.*.log", "system.info.security.log", false},
	{"log.*.*", "log.warning.high", true},
	{"log.*.*", "log.warning", false},

	// '#' at the middle
	{"app.#.error", "app.server.db.error", true},
	{"app.#.error", "app.server.db.primary.error", true},
	{"app.#.error", "app.server.db.primary", false},
	{"log.#.warn", "log.system.warn", true},
	{"log.#.warn", "warn.system", false},

	// '#' patters more
	{"user.#.update", "user.profile.update", true},
	{"user.#.update", "user.settings.update", true},
	{"user.#.update", "user.update", true}, // '#' はゼロ個でもOK
	{"user.#.update", "user.profile.name.update", true},
	{"user.#.update", "user.settings.profile", false},

	// '*' and '#' are mixed
	{"app.*.#.error", "app.api.db.error", true},
	{"app.*.#.error", "app.api.server.db.error", true},
	{"app.*.#.error", "app.api.server.warning", false},
	{"app.*.#.critical", "app.api.db.critical", true},
	{"app.*.#.critical", "app.api.server.critical.error", false},

	// edge case
	{"*", "test", true},
	{"*", "multi.word.test", false},
	{"#.#", "a.b.c.d.e", true},
	{"#.#", "single", false},
	{"a.#.b.#.c", "a.x.y.b.z.c", true},
	{"a.#.b.#.c", "a.b.c", true},
	{"a.#.b.#.c", "a.b.x.y", false},

	// % wildcard tests (token-level substring matching)
	{"foo.*.a%", "foo.yyy.aaa", true},        // * matches "yyy", a% matches "aaa"
	{"foo.*.a%", "foo.yyy.app", true},        // * matches "yyy", a% matches "app"
	{"foo.*.a%", "foo.yyy.bbb", false},       // * matches "yyy", a% doesn't match "bbb"
	{"app.%service", "app.webservice", true}, // %service matches "webservice"
	{"app.%service", "app.apiservice", true}, // %service matches "apiservice"
	{"app.%service", "app.service", true},    // %service matches "service" (% matches empty)
	{"app.%service", "app.server", false},    // %service doesn't match "server"
	{"%.server.*", "web.server.01", true},    // % matches "web", * matches "01"
	{"%.server.*", "api.server.prod", true},  // % matches "api", * matches "prod"
	{"%.server.*", "server.01", false},       // % needs something to match (dot separation)

	// Multiple % in same token
	{"app.%-%", "app.web-api", true}, // %-%  matches "web-api"
	{"app.%-%", "app.single", false}, // %-% needs a dash

	// % with # wildcard (cross-token)
	{"app.%.#", "app.service.log.error", true}, // % in token, # matches rest
	{"app.%.#", "app.service", true},           // % matches "service", # matches nothing

	// Edge cases
	{"%.%", "a.b", true},    // % matches "a", % matches "b"
	{"%.%", ".", true},      // % matches empty token on each side
	{"%", "anything", true}, // single token with %
	{"app.%", "app.", true}, // % matches empty token
}

func TestMatch(t *testing.T) {
	t.Parallel()
	for _, test := range matchTests {
		t.Run(fmt.Sprintf("%s_%s_%v", test.pattern, test.key, test.want), func(t *testing.T) {
			t.Parallel()
			result := Match(test.key, test.pattern)
			if result != test.want {
				t.Errorf("pattern %s and key %s: got %t, want %t", test.pattern, test.key, result, test.want)
			}
		})
	}
}
