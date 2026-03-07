package monitor

import (
	"testing"
	"time"
)

func TestCachedTmuxResolver_CachesWithinTTL(t *testing.T) {
	base := time.Unix(100, 0)
	resolver := &TmuxResolver{targetByPID: map[int]string{1: "main:0.0"}}
	calls := 0

	m := &Monitor{
		newTmuxResolver: func() *TmuxResolver {
			calls++
			return resolver
		},
		tmuxResolverTTL: 5 * time.Second,
	}

	got1 := m.cachedTmuxResolver(base)
	got2 := m.cachedTmuxResolver(base.Add(4 * time.Second))

	if calls != 1 {
		t.Fatalf("newTmuxResolver calls = %d, want 1", calls)
	}
	if got1 != resolver || got2 != resolver {
		t.Fatal("cachedTmuxResolver should return the cached resolver")
	}
}

func TestCachedTmuxResolver_RefreshesAfterTTL(t *testing.T) {
	base := time.Unix(200, 0)
	calls := 0

	m := &Monitor{
		newTmuxResolver: func() *TmuxResolver {
			calls++
			return &TmuxResolver{targetByPID: map[int]string{calls: "main:0.0"}}
		},
		tmuxResolverTTL: 5 * time.Second,
	}

	got1 := m.cachedTmuxResolver(base)
	got2 := m.cachedTmuxResolver(base.Add(5 * time.Second))

	if calls != 2 {
		t.Fatalf("newTmuxResolver calls = %d, want 2", calls)
	}
	if got1 == nil || got2 == nil {
		t.Fatal("cachedTmuxResolver should return non-nil resolvers from the test factory")
	}
	if got1 == got2 {
		t.Fatal("resolver should refresh after TTL expiry")
	}
}

func TestCachedTmuxResolver_CachesNilWithinTTL(t *testing.T) {
	base := time.Unix(300, 0)
	calls := 0

	m := &Monitor{
		newTmuxResolver: func() *TmuxResolver {
			calls++
			return nil
		},
		tmuxResolverTTL: 5 * time.Second,
	}

	got1 := m.cachedTmuxResolver(base)
	got2 := m.cachedTmuxResolver(base.Add(2 * time.Second))

	if calls != 1 {
		t.Fatalf("newTmuxResolver calls = %d, want 1", calls)
	}
	if got1 != nil || got2 != nil {
		t.Fatal("cachedTmuxResolver should return nil when tmux is unavailable")
	}
}

func TestCachedTmuxResolver_DisabledTTLNoCache(t *testing.T) {
	base := time.Unix(400, 0)
	calls := 0

	m := &Monitor{
		newTmuxResolver: func() *TmuxResolver {
			calls++
			return &TmuxResolver{targetByPID: map[int]string{calls: "main:0.0"}}
		},
		tmuxResolverTTL: 0,
	}

	_ = m.cachedTmuxResolver(base)
	_ = m.cachedTmuxResolver(base.Add(time.Second))

	if calls != 2 {
		t.Fatalf("newTmuxResolver calls = %d, want 2 when cache is disabled", calls)
	}
}
