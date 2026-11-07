package agentpolicy

import (
	"strings"
	"testing"
)

func TestNativeProfilesCannotBeEnabledWithoutEvidence(t *testing.T) {
	for _, p := range Defaults() {
		p.Blocker = ""
		for _, op := range p.Operations {
			if e := p.Check(op); e == nil || e.Error() != "policy_unenforceable" {
				t.Fatal(p.AgentID, op, e)
			}
		}
	}
	cfg := DefaultConfig()
	if cfg.DailyDispatches != 0 || cfg.DailyProcessStarts != 0 {
		t.Fatal("nonzero default budget")
	}
	for _, host := range []string{"0.0.0.0", "localhost", "127.0.0.2", "192.168.1.1"} {
		cfg.Host = host
		if cfg.Validate() == nil {
			t.Fatal("accepted host", host)
		}
	}
}
func TestMinimumEnvironmentDropsInjectedCredentials(t *testing.T) {
	env := Environment([]string{"OPENAI_API_KEY=secret", "Authorization=Bearer secret", "HTTP_PROXY=http://evil", "DYLD_INSERT_LIBRARIES=evil", "BASH_ENV=evil", "CODEX_HOME=/private/native", "HOME=/private/real", "XDG_CONFIG_HOME=/private/config"}, "/synthetic/home")
	joined := strings.Join(env, "\n")
	for _, s := range []string{"secret", "evil", "/private"} {
		if strings.Contains(joined, s) {
			t.Fatal(joined)
		}
	}
}
