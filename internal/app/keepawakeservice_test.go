package app

import (
	"context"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
)

func newKeepAwakeTest(t *testing.T) (*KeepAwakeService, *fakeRuntime, *keepawake.FakeHolder, *keepawake.FakeClock) {
	t.Helper()
	h, clk, rt := &keepawake.FakeHolder{}, keepawake.NewFakeClock(), &fakeRuntime{}
	m := keepawake.New(keepawake.Options{Holders: []keepawake.Holder{h}, Clock: clk, Config: keepawake.DefaultConfig()})
	s := NewKeepAwakeService(m, rt)
	if err := s.ServiceStartup(context.Background(), applicationServiceOptions()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.ServiceShutdown() })
	return s, rt, h, clk
}

func TestKeepAwakeServiceStateEventsAndConfig(t *testing.T) {
	s, rt, h, _ := newKeepAwakeTest(t)
	st, err := s.GetState()
	if err != nil || st.Held || st.Config != keepawake.DefaultConfig() {
		t.Fatalf("%+v %v", st, err)
	}
	s.manager().SetState("a", keepawake.KindAI, keepawake.StateRunning)
	if held, _ := h.Held(); !held {
		t.Fatal("not held")
	}
	var last api.KeepAwakeState
	rt.mu.Lock()
	for _, e := range rt.events {
		if e.Name == api.EventKeepAwakeChanged {
			last = e.Data.(api.KeepAwakeState)
		}
	}
	rt.mu.Unlock()
	if !last.Held || last.Running != 1 {
		t.Fatalf("event %+v", last)
	}

	cfg, _ := s.GetConfig()
	cfg.Enabled = false
	st, err = s.SetConfig(cfg)
	if err != nil || st.Held || st.Config.Enabled {
		t.Fatalf("%+v %v", st, err)
	}
	if held, _ := h.Held(); held {
		t.Fatal("still held after the setting was turned off")
	}
	cfg.CeilingHours = 5
	if _, err := s.SetConfig(cfg); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatalf("err %v", err)
	}
}

func TestKeepAwakeServiceShutdownReleases(t *testing.T) {
	s, _, h, _ := newKeepAwakeTest(t)
	s.manager().SetState("a", keepawake.KindAI, keepawake.StateRunning)
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if held, _ := h.Held(); held {
		t.Fatal("held after shutdown")
	}
}
