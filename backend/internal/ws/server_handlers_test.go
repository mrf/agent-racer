package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/session"
)

// newHandlerTestServer creates a Server with a real store and broadcaster,
// suitable for handler-level tests via httptest.NewRecorder.
func newHandlerTestServer(t *testing.T, authToken string) *Server {
	t.Helper()
	store := session.NewStore()
	broadcaster := NewBroadcaster(store, 10*time.Millisecond, time.Second, 10)
	t.Cleanup(func() { broadcaster.Stop() })
	cfg := &config.Config{
		Sound: config.SoundConfig{
			Enabled:       true,
			MasterVolume:  0.8,
			AmbientVolume: 0.5,
			SfxVolume:     0.7,
			EnableAmbient: true,
			EnableSfx:     true,
		},
	}
	return NewServer(cfg, store, broadcaster, "", false, nil, nil, authToken)
}

// newTrackerForTest creates a StatsTracker backed by a temp directory.
func newTrackerForTest(t *testing.T) *gamification.StatsTracker {
	t.Helper()
	dir := t.TempDir()
	persist := gamification.NewStore(dir)
	tracker, _, err := gamification.NewStatsTracker(persist, 16, nil)
	if err != nil {
		t.Fatalf("NewStatsTracker: %v", err)
	}
	return tracker
}

// authReq builds an httptest request with an optional Bearer token and body.
func authReq(method, url, token string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// newTailLogFile creates a temporary JSONL file under ~/.claude/projects/ so
// it passes ValidateLogPath. Returns the file path. The directory is cleaned
// up automatically via t.Cleanup.
func newTailLogFile(t *testing.T, dirSuffix string, content string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home dir: %v", err)
	}
	logDir := filepath.Join(home, ".claude", "projects", dirSuffix)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(logDir) })

	logFile := filepath.Join(logDir, "test.jsonl")
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return logFile
}

// ─── handleSessions ──────────────────────────────────────────────────────────

func TestHandleSessions_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleSessions(rec, authReq(http.MethodGet, "/api/sessions", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleSessions_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleSessions(rec, authReq(http.MethodPost, "/api/sessions", "", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleSessions_Empty(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleSessions(rec, authReq(http.MethodGet, "/api/sessions", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var sessions []*session.SessionState
	if err := json.NewDecoder(rec.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("got %d sessions, want 0", len(sessions))
	}
}

func TestHandleSessions_ReturnsSessions(t *testing.T) {
	s := newHandlerTestServer(t, "tok")
	s.store.Update(&session.SessionState{
		ID:     "sess-1",
		Source: "claude",
	})
	rec := httptest.NewRecorder()
	s.handleSessions(rec, authReq(http.MethodGet, "/api/sessions", "tok", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var sessions []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0]["id"] != "sess-1" {
		t.Errorf("session id = %q, want %q", sessions[0]["id"], "sess-1")
	}
}

// ─── handleConfig ────────────────────────────────────────────────────────────

func TestHandleConfig_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleConfig(rec, authReq(http.MethodGet, "/api/config", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleConfig_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleConfig(rec, authReq(http.MethodPost, "/api/config", "", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleConfig_ReturnsSoundConfig(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleConfig(rec, authReq(http.MethodGet, "/api/config", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var sound config.SoundConfig
	if err := json.NewDecoder(rec.Body).Decode(&sound); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !sound.Enabled {
		t.Error("sound.Enabled = false, want true")
	}
	if sound.MasterVolume != 0.8 {
		t.Errorf("sound.MasterVolume = %v, want 0.8", sound.MasterVolume)
	}
}

// ─── handleStats ─────────────────────────────────────────────────────────────

func TestHandleStats_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleStats(rec, authReq(http.MethodGet, "/api/stats", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleStats_NilTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleStats(rec, authReq(http.MethodGet, "/api/stats", "", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleStats_WithTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	tracker := newTrackerForTest(t)
	s.SetStatsTracker(tracker)

	rec := httptest.NewRecorder()
	s.handleStats(rec, authReq(http.MethodGet, "/api/stats", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var stats map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := stats["totalSessions"]; !ok {
		t.Error("missing totalSessions field")
	}
}

// ─── handleAchievements ──────────────────────────────────────────────────────

func TestHandleAchievements_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleAchievements(rec, authReq(http.MethodGet, "/api/achievements", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleAchievements_NoTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleAchievements(rec, authReq(http.MethodGet, "/api/achievements", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var achievements []achievementResponse
	if err := json.NewDecoder(rec.Body).Decode(&achievements); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Without a tracker, all achievements should be listed but none unlocked.
	if len(achievements) == 0 {
		t.Fatal("expected non-empty achievement list")
	}
	for _, a := range achievements {
		if a.Unlocked {
			t.Errorf("achievement %q should not be unlocked without tracker", a.ID)
		}
	}
}

func TestHandleAchievements_WithUnlocked(t *testing.T) {
	s := newHandlerTestServer(t, "")
	tracker := newTrackerForTest(t)
	s.SetStatsTracker(tracker)

	rec := httptest.NewRecorder()
	s.handleAchievements(rec, authReq(http.MethodGet, "/api/achievements", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var achievements []achievementResponse
	if err := json.NewDecoder(rec.Body).Decode(&achievements); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(achievements) == 0 {
		t.Fatal("expected non-empty achievement list")
	}
	a := achievements[0]
	if a.ID == "" || a.Name == "" || a.Tier == "" || a.Category == "" {
		t.Errorf("achievement missing required fields: %+v", a)
	}
}

// ─── handleEquip ─────────────────────────────────────────────────────────────

func TestHandleEquip_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodGet, "/api/equip", "", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleEquip_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"x","slot":"paint"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleEquip_NilTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"x","slot":"paint"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleEquip_MissingRewardID(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"slot":"paint"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleEquip_MissingSlot(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"x"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleEquip_InvalidSlot(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"x","slot":"invalid"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleEquip_UnknownReward(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"nonexistent","slot":"paint"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleEquip_SlotMismatch(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	// "spark_trail" is a trail reward, but we request slot "paint"
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"spark_trail","slot":"paint"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "slot mismatch") {
		t.Errorf("body = %q, want 'slot mismatch'", rec.Body.String())
	}
}

func TestHandleEquip_NotUnlocked(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	// "rookie_paint" is unlocked by "first_lap" which is not yet achieved
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `{"rewardId":"rookie_paint","slot":"paint"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleEquip_InvalidBody(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleEquip(rec, authReq(http.MethodPost, "/api/equip", "", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ─── handleUnequip ───────────────────────────────────────────────────────────

func TestHandleUnequip_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleUnequip(rec, authReq(http.MethodGet, "/api/unequip", "", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleUnequip_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleUnequip(rec, authReq(http.MethodPost, "/api/unequip", "", `{"slot":"paint"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleUnequip_NilTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleUnequip(rec, authReq(http.MethodPost, "/api/unequip", "", `{"slot":"paint"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleUnequip_MissingSlot(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleUnequip(rec, authReq(http.MethodPost, "/api/unequip", "", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleUnequip_InvalidSlot(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleUnequip(rec, authReq(http.MethodPost, "/api/unequip", "", `{"slot":"banana"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleUnequip_Success(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleUnequip(rec, authReq(http.MethodPost, "/api/unequip", "", `{"slot":"paint"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var loadout gamification.Equipped
	if err := json.NewDecoder(rec.Body).Decode(&loadout); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if loadout.Paint != "" {
		t.Errorf("paint = %q, want empty", loadout.Paint)
	}
}

// ─── handleTail ──────────────────────────────────────────────────────────────

func TestHandleTail_SessionNotFound(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	req := authReq(http.MethodGet, "/api/sessions/nonexistent/tail", "", "")
	s.handleTail(rec, req, "nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTail_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", LogPath: "/tmp/test.jsonl"})
	rec := httptest.NewRecorder()
	req := authReq(http.MethodPost, "/api/sessions/s1/tail", "", "")
	s.handleTail(rec, req, "s1")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleTail_NoLogPath(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", LogPath: ""})
	rec := httptest.NewRecorder()
	req := authReq(http.MethodGet, "/api/sessions/s1/tail", "", "")
	s.handleTail(rec, req, "s1")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleTail_WithOffsetAndLimit(t *testing.T) {
	line := `{"type":"system","subtype":"init","timestamp":"2025-01-01T00:00:00Z"}` + "\n"
	logFile := newTailLogFile(t, "test-handler-tail", line+line+line)

	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", LogPath: logFile})

	rec := httptest.NewRecorder()
	req := authReq(http.MethodGet, "/api/sessions/s1/tail?offset=0&limit=1", "", "")
	s.handleTail(rec, req, "s1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp session.TailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Errorf("got %d entries, want 1", len(resp.Entries))
	}
	if resp.Offset <= 0 {
		t.Errorf("offset = %d, want > 0", resp.Offset)
	}
}

func TestHandleTail_InvalidOffsetIgnored(t *testing.T) {
	line := `{"type":"system","subtype":"init","timestamp":"2025-01-01T00:00:00Z"}` + "\n"
	logFile := newTailLogFile(t, "test-handler-tail-inv", line)

	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", LogPath: logFile})

	// Invalid offset should be treated as 0 (not an error).
	rec := httptest.NewRecorder()
	req := authReq(http.MethodGet, "/api/sessions/s1/tail?offset=abc", "", "")
	s.handleTail(rec, req, "s1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ─── handleFocus ─────────────────────────────────────────────────────────────

func TestHandleFocus_SessionNotFound(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	req := authReq(http.MethodPost, "/api/sessions/nonexistent/focus", "", "")
	s.handleFocus(rec, req, "nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleFocus_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", TmuxTarget: "main:0.0"})
	rec := httptest.NewRecorder()
	req := authReq(http.MethodGet, "/api/sessions/s1/focus", "", "")
	s.handleFocus(rec, req, "s1")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleFocus_NoTmuxTarget(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", TmuxTarget: ""})
	rec := httptest.NewRecorder()
	req := authReq(http.MethodPost, "/api/sessions/s1/focus", "", "")
	s.handleFocus(rec, req, "s1")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleFocus_TmuxError(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.store.Update(&session.SessionState{ID: "s1", TmuxTarget: "nosuchsession:99.99"})
	rec := httptest.NewRecorder()
	req := authReq(http.MethodPost, "/api/sessions/s1/focus", "", "")
	s.handleFocus(rec, req, "s1")
	// 500 when tmux is unavailable (CI), 204 if tmux happens to be running.
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d or %d", rec.Code, http.StatusInternalServerError, http.StatusNoContent)
	}
}

// ─── handleSessionRoutes ─────────────────────────────────────────────────────

func TestHandleSessionRoutes_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleSessionRoutes(rec, authReq(http.MethodGet, "/api/sessions/s1/tail", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleSessionRoutes_InvalidPath(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleSessionRoutes(rec, authReq(http.MethodGet, "/api/sessions/s1", "", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSessionRoutes_UnknownAction(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleSessionRoutes(rec, authReq(http.MethodGet, "/api/sessions/s1/unknown", "", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ─── handleChallenges ────────────────────────────────────────────────────────

func TestHandleChallenges_NoAuth(t *testing.T) {
	s := newHandlerTestServer(t, "secret")
	rec := httptest.NewRecorder()
	s.handleChallenges(rec, authReq(http.MethodGet, "/api/challenges", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleChallenges_NilTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleChallenges(rec, authReq(http.MethodGet, "/api/challenges", "", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleChallenges_WithTracker(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetStatsTracker(newTrackerForTest(t))
	rec := httptest.NewRecorder()
	s.handleChallenges(rec, authReq(http.MethodGet, "/api/challenges", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ─── handleHealthz ───────────────────────────────────────────────────────────

func TestHandleHealthz_OK(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, authReq(http.MethodGet, "/healthz", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp healthzResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.UptimeSeconds <= 0 {
		t.Error("expected positive uptimeSeconds")
	}
}

func TestHandleHealthz_MethodNotAllowed(t *testing.T) {
	s := newHandlerTestServer(t, "")
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, authReq(http.MethodPost, "/healthz", "", ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleHealthz_DegradedSource(t *testing.T) {
	s := newHandlerTestServer(t, "")
	s.SetHealthHook(func() []SourceHealthPayload {
		return []SourceHealthPayload{
			{Source: "claude", Status: StatusFailed, LastError: "timeout"},
		}
	})
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, authReq(http.MethodGet, "/healthz", "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp healthzResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want %q", resp.Status, "degraded")
	}
	if len(resp.Sources) != 1 {
		t.Errorf("sources count = %d, want 1", len(resp.Sources))
	}
}

// ─── decodeBody ──────────────────────────────────────────────────────────────

func TestDecodeBody_Valid(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"slot":"paint"}`))
	var dst unequipRequest
	if !decodeBody(rec, req, &dst) {
		t.Fatal("decodeBody returned false for valid body")
	}
	if dst.Slot != "paint" {
		t.Errorf("slot = %q, want %q", dst.Slot, "paint")
	}
}

func TestDecodeBody_InvalidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{bad`))
	var dst unequipRequest
	if decodeBody(rec, req, &dst) {
		t.Fatal("decodeBody returned true for invalid JSON")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDecodeBody_TooLarge(t *testing.T) {
	rec := httptest.NewRecorder()
	// Create a valid-looking JSON body larger than maxRequestBodySize (1 MB).
	// The JSON decoder must read past the limit to trigger MaxBytesError.
	big := `{"slot":"` + strings.Repeat("a", 1<<20+100) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	var dst unequipRequest
	if decodeBody(rec, req, &dst) {
		t.Fatal("decodeBody returned true for oversized body")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

// ─── authorize ───────────────────────────────────────────────────────────────

func TestAuthorize_NoToken(t *testing.T) {
	s := newHandlerTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !s.authorize(req) {
		t.Error("authorize should return true when no auth token configured")
	}
}

func TestAuthorize_ValidToken(t *testing.T) {
	s := newHandlerTestServer(t, "my-secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-secret")
	if !s.authorize(req) {
		t.Error("authorize should return true for valid token")
	}
}

func TestAuthorize_InvalidToken(t *testing.T) {
	s := newHandlerTestServer(t, "my-secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	if s.authorize(req) {
		t.Error("authorize should return false for invalid token")
	}
}

func TestAuthorize_MissingHeader(t *testing.T) {
	s := newHandlerTestServer(t, "my-secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if s.authorize(req) {
		t.Error("authorize should return false when no Authorization header")
	}
}

func TestAuthorize_WrongScheme(t *testing.T) {
	s := newHandlerTestServer(t, "my-secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic my-secret")
	if s.authorize(req) {
		t.Error("authorize should return false for non-Bearer scheme")
	}
}

// ─── writeRateLimitExceeded ──────────────────────────────────────────────────

func TestWriteRateLimitExceeded(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRateLimitExceeded(rec, 30*time.Second)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "30" {
		t.Errorf("Retry-After = %q, want %q", ra, "30")
	}
}

func TestWriteRateLimitExceeded_SubSecondCeils(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRateLimitExceeded(rec, 500*time.Millisecond)
	if ra := rec.Header().Get("Retry-After"); ra != "1" {
		t.Errorf("Retry-After = %q, want %q", ra, "1")
	}
}

// ─── HSTS headers ────────────────────────────────────────────────────────────

func TestHSTSHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	hstsHeaders(inner).ServeHTTP(rec, req)
	hsts := rec.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatal("missing Strict-Transport-Security header")
	}
	if !strings.Contains(hsts, "max-age=63072000") {
		t.Errorf("HSTS = %q, want max-age=63072000", hsts)
	}
}
