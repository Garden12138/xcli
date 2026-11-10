package modelstore

import (
	"strings"
	"testing"
)

func TestSIWCIsReservedAndCannotBeEnabledOrUseAPIKey(t *testing.T) {
	for _, endpoint := range []string{"https://api.openai.com/v1", "http://127.0.0.1:1455/v1", "https://attacker.invalid/v1"} {
		config := strings.Replace(fixtureConfig, "openai-compatible", "siwc-responses", 1)
		config = strings.Replace(config, "http://127.0.0.1:11434/v1", endpoint, 1)
		for _, key := range []string{"", "    api_key_ref: env:SYNTHETIC_NEVER_READ\n"} {
			_, err := Decode([]byte(strings.Replace(config, "    type: siwc-responses\n", "    type: siwc-responses\n"+key, 1)))
			if err == nil || !strings.Contains(err.Error(), "siwc_eligibility_blocked") {
				t.Fatal("SIWC production gate bypassed", err)
			}
		}
	}
}
