package tracks

import "testing"

func TestPresetsReturnsThreeTracks(t *testing.T) {
	presets := Presets()
	if len(presets) != 3 {
		t.Fatalf("len(Presets()) = %d, want 3", len(presets))
	}
}

func TestPresetsHaveValidIDs(t *testing.T) {
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		p := presets[i]
		if p.ID == "" {
			t.Fatalf("preset[%d] has empty ID", i)
		}
		if !validID.MatchString(p.ID) {
			t.Fatalf("preset %q has invalid ID", p.ID)
		}
	}
}

func TestPresetsHaveNonEmptyNames(t *testing.T) {
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		if presets[i].Name == "" {
			t.Fatalf("preset %q has empty Name", presets[i].ID)
		}
	}
}

func TestPresetTileGridMatchesDimensions(t *testing.T) {
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		p := presets[i]
		if p.Width <= 0 || p.Height <= 0 {
			t.Fatalf("preset %q has invalid dimensions: %dx%d", p.ID, p.Width, p.Height)
		}
		if len(p.Tiles) != p.Height {
			t.Fatalf("preset %q: len(Tiles) = %d, want Height %d", p.ID, len(p.Tiles), p.Height)
		}
		for row := 0; row < len(p.Tiles); row++ {
			if len(p.Tiles[row]) != p.Width {
				t.Fatalf("preset %q: len(Tiles[%d]) = %d, want Width %d", p.ID, row, len(p.Tiles[row]), p.Width)
			}
		}
	}
}

func TestPresetsHaveTimestamps(t *testing.T) {
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		p := presets[i]
		if p.CreatedAt.IsZero() {
			t.Fatalf("preset %q has zero CreatedAt", p.ID)
		}
		if p.UpdatedAt.IsZero() {
			t.Fatalf("preset %q has zero UpdatedAt", p.ID)
		}
	}
}

func TestPresetsHaveUniqueIDs(t *testing.T) {
	presets := Presets()
	seen := make(map[string]bool)
	for i := 0; i < len(presets); i++ {
		if seen[presets[i].ID] {
			t.Fatalf("duplicate preset ID: %q", presets[i].ID)
		}
		seen[presets[i].ID] = true
	}
}

func TestIsPresetForKnownIDs(t *testing.T) {
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		if !isPreset(presets[i].ID) {
			t.Fatalf("isPreset(%q) = false, want true", presets[i].ID)
		}
	}
}

func TestIsPresetForUnknownID(t *testing.T) {
	if isPreset("not-a-preset") {
		t.Fatal("isPreset(\"not-a-preset\") = true, want false")
	}
	if isPreset("") {
		t.Fatal("isPreset(\"\") = true, want false")
	}
}

func TestExpectedPresetIDs(t *testing.T) {
	expected := []string{"oval", "figure8", "f1-circuit"}
	presets := Presets()
	if len(presets) != len(expected) {
		t.Fatalf("len(Presets()) = %d, want %d", len(presets), len(expected))
	}
	for i := 0; i < len(expected); i++ {
		if presets[i].ID != expected[i] {
			t.Fatalf("Presets()[%d].ID = %q, want %q", i, presets[i].ID, expected[i])
		}
	}
}
