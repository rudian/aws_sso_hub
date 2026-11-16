package app

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestOnlineHonoursOverride(t *testing.T) {
	c := &Container{opts: ContainerOptions{Online: func() bool { return false }}}
	if c.online() {
		t.Fatal("override false ignored")
	}
	c.opts.Online = func() bool { return true }
	if !c.online() {
		t.Fatal("override true ignored")
	}
}

func cidr(t *testing.T, s string) net.Addr {
	t.Helper()
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	n.IP = ip
	return n
}

func TestHasUsableAddr(t *testing.T) {
	cases := []struct {
		name  string
		addrs []net.Addr
		want  bool
	}{
		{"none", nil, false},
		{"ipv6 link-local only (macOS utun)", []net.Addr{cidr(t, "fe80::1/64")}, false},
		{"ipv4 link-local only", []net.Addr{cidr(t, "169.254.3.4/16")}, false},
		{"loopback only", []net.Addr{cidr(t, "127.0.0.1/8"), cidr(t, "::1/128")}, false},
		{"unix-ish non-IPNet", []net.Addr{&net.IPAddr{IP: net.ParseIP("10.0.0.2")}}, false},
		{"private ipv4", []net.Addr{cidr(t, "192.168.1.20/24")}, true},
		{"link-local plus global", []net.Addr{cidr(t, "fe80::1/64"), cidr(t, "2001:db8::5/64")}, true},
	}
	for _, tc := range cases {
		if got := hasUsableAddr(tc.addrs); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func fetchContainer(online bool, calls *atomic.Int32) *Container {
	return &Container{
		opts: ContainerOptions{Online: func() bool { return online }},
		fetchRound: func(ctx context.Context) (int, int) {
			calls.Add(1)
			return 1, 0
		},
	}
}

func TestFetchLoopSkipsWhileOffline(t *testing.T) {
	var calls atomic.Int32
	c := fetchContainer(false, &calls)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	c.fetchLoop(ctx, 5*time.Millisecond)
	if n := calls.Load(); n != 0 {
		t.Fatalf("fetched %d times while offline", n)
	}
}

func TestFetchLoopFetchesWhileOnline(t *testing.T) {
	var calls atomic.Int32
	c := fetchContainer(true, &calls)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	c.fetchLoop(ctx, 5*time.Millisecond)
	if calls.Load() < 2 {
		t.Fatalf("expected repeated fetches, got %d", calls.Load())
	}
}

func TestFetchLoopBoundsEachRound(t *testing.T) {
	var calls atomic.Int32
	c := &Container{
		opts: ContainerOptions{Online: func() bool { return true }},
		fetchRound: func(ctx context.Context) (int, int) {
			calls.Add(1)
			<-ctx.Done() // a hung git: only the round deadline ends it
			return 0, 1
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	c.fetchLoop(ctx, 10*time.Millisecond)
	if calls.Load() < 2 {
		t.Fatalf("a hung round stopped the loop: %d calls", calls.Load())
	}
}
