package orchestrator

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/proc"
	"github.com/Garden12138/xcli/internal/testutil"
)

func TestRouterOrdersByStrength(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{})
	r := NewRouter(e)
	for _, tc := range []struct {
		kind api.Kind
		want []string
	}{
		{api.KindAsk, []string{"codex", "cursor", "antigravity"}},
		{api.KindImage, []string{"codex", "antigravity", "cursor"}}, {api.KindReview, []string{"cursor", "codex", "antigravity"}},
	} {
		if got := r.Candidates(tc.kind); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestRouterRespectsAllowedOrder(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{Agents: []string{"cursor", "codex"}})
	if got := NewRouter(e).Candidates(api.KindAsk); !reflect.DeepEqual(got, []string{"cursor", "codex"}) {
		t.Fatal(got)
	}
}

func TestRouterSkipsCooldownAndMissing(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{})
	e.Health.Cooldown("codex", driver.ErrQuota, time.Now())
	path, err := proc.LookPath("agy", e.PathList)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := NewRouter(e).Candidates(api.KindCode); !reflect.DeepEqual(got, []string{"cursor"}) {
		t.Fatal(got)
	}
}

func TestRouterExclude(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{})
	if got := NewRouter(e).Candidates(api.KindReview, "cursor"); !reflect.DeepEqual(got, []string{"codex", "antigravity"}) {
		t.Fatal(got)
	}
}

func TestRouterLockedAgent(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{Agent: "codex"})
	if got := NewRouter(e).Candidates(api.KindImage); !reflect.DeepEqual(got, []string{"codex"}) {
		t.Fatal(got)
	}
	if got := NewRouter(e).Candidates(api.KindReview, "codex"); len(got) != 0 {
		t.Fatal("locked agent was selected to review itself")
	}
}
