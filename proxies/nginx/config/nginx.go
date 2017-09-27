package config

import (
	"github.com/caicloud/canary-release/pkg/api"
	nginx "k8s.io/ingress/controllers/nginx/pkg/config"
)

type TemplateConfig struct {
	MaxOpenFiles  int
	BacklogSize   int
	IsIPV6Enabled bool
	Cfg           nginx.Configuration
	TCPBackends   []api.L4Service
	UDPBackends   []api.L4Service
}

func NewDefaultTemplateConfig() TemplateConfig {
	return TemplateConfig{
		Cfg: nginx.NewDefault(),
	}
}

// Equal tests for equality between tow Template Config types
func (c *TemplateConfig) Equal(c2 *TemplateConfig) bool {
	if c == c2 {
		return true
	}
	if c == nil || c2 == nil {
		return false
	}

	if len(c.TCPBackends) != len(c2.TCPBackends) {
		return false
	}

	for _, c1b := range c.TCPBackends {
		found := false
		for _, c2b := range c2.TCPBackends {
			if c1b.Equal(c2b) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(c.UDPBackends) != len(c2.UDPBackends) {
		return false
	}

	for _, c1b := range c.UDPBackends {
		found := false
		for _, c2b := range c2.UDPBackends {
			if c1b.Equal(c2b) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
