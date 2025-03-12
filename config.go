package trabbits

// Config represents the configuration of the trabbits proxy.
type Config struct {
	Upstream UpstreamConfig `yaml:"upstream" json:"upstream"`
	Auth     []AuthConfig   `yaml:"auth" json:"auth"`
}

// AuthConfig represents a user and password pair for PLAIN authentication.
type AuthConfig struct {
	User string `yaml:"user" json:"user"`
	Pass string `yaml:"pass" json:"pass"`
}

// UpstreamConfig represents the configuration of an upstream server.
type UpstreamConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}
