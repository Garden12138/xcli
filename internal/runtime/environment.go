package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Garden12138/xcli/internal/config"
)

func BuildEnvironment(parent []string, cfg config.Config, agent config.AgentConfig, networkOverride string) ([]string, error) {
	values := make(map[string]string, len(parent))
	for _, entry := range parent {
		index := strings.IndexByte(entry, '=')
		if index < 0 {
			continue
		}
		values[entry[:index]] = entry[index+1:]
	}

	networkName := agent.Network
	if networkOverride != "" {
		networkName = networkOverride
	}
	if networkName != "" {
		network, ok := cfg.Networks[networkName]
		if !ok {
			return nil, fmt.Errorf("unknown network %q", networkName)
		}
		for _, key := range network.Unset {
			delete(values, key)
		}
		for key, value := range network.Set {
			values[key] = value
		}
	}
	for key, value := range agent.Env {
		values[key] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment, nil
}
