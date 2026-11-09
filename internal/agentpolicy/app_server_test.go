package agentpolicy

import "testing"

func TestAppServerFlagDoesNotPromoteNative(t *testing.T) {
	cfg := DefaultConfig()
	base := Defaults()
	if len(BindAppServer(t.TempDir(), cfg, base)) != len(base) {
		t.Fatal("flag enabled by default")
	}
	cfg.CodexAppServerEnabled = true
	profiles := BindAppServer(t.TempDir(), cfg, base)
	if len(profiles) != len(base)+1 {
		t.Fatal("missing candidate")
	}
	p := profiles[len(profiles)-1]
	p.Blocker = ""
	if p.AdapterID != AppServerAdapter || p.Check("text_task") == nil {
		t.Fatal("flag/control cleared native boundary")
	}
	if p.Check("image_generate") == nil {
		t.Fatal("media exposed")
	}
	if base[0].Check("text_task") == nil {
		t.Fatal("legacy exec enabled")
	}
}
