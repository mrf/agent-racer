package tracks

import "time"

// Presets returns the 3 built-in track layouts.
func Presets() []*Track {
	return []*Track{
		ovalTrack(),
		figure8Track(),
		f1Track(),
	}
}

func emptyGrid(w, h int) [][]string {
	grid := make([][]string, h)
	for i := 0; i < h; i++ {
		grid[i] = make([]string, w)
	}
	return grid
}

func ovalTrack() *Track {
	w, h := 32, 16
	g := emptyGrid(w, h)
	for c := 5; c <= 26; c++ {
		g[2][c] = "straight-h"
		g[13][c] = "straight-h"
	}
	for r := 3; r <= 12; r++ {
		g[r][4] = "straight-v"
		g[r][27] = "straight-v"
	}
	g[2][4] = "curve-sw"
	g[2][27] = "curve-se"
	g[13][4] = "curve-nw"
	g[13][27] = "curve-ne"
	g[7][4] = "start-line"
	g[8][4] = "finish-line"
	return &Track{
		ID: "oval", Name: "Oval Circuit",
		Width: w, Height: h, Tiles: g,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func figure8Track() *Track {
	w, h := 32, 16
	g := emptyGrid(w, h)
	for c := 3; c <= 13; c++ {
		g[1][c] = "straight-h"
		g[7][c] = "straight-h"
	}
	for r := 2; r <= 6; r++ {
		g[r][2] = "straight-v"
		g[r][14] = "straight-v"
	}
	g[1][2] = "curve-sw"
	g[1][14] = "curve-se"
	g[7][2] = "curve-nw"
	g[7][14] = "curve-ne"
	for c := 18; c <= 28; c++ {
		g[8][c] = "straight-h"
		g[14][c] = "straight-h"
	}
	for r := 9; r <= 13; r++ {
		g[r][17] = "straight-v"
		g[r][29] = "straight-v"
	}
	g[8][17] = "curve-sw"
	g[8][29] = "curve-se"
	g[14][17] = "curve-nw"
	g[14][29] = "curve-ne"
	g[7][15] = "straight-h"
	g[7][16] = "straight-h"
	g[8][15] = "straight-h"
	g[8][16] = "straight-h"
	g[4][2] = "start-line"
	return &Track{
		ID: "figure8", Name: "Figure-8",
		Width: w, Height: h, Tiles: g,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func f1Track() *Track {
	w, h := 32, 16
	g := emptyGrid(w, h)
	for c := 5; c <= 20; c++ {
		g[12][c] = "straight-h"
	}
	g[12][5] = "start-line"
	g[12][20] = "finish-line"
	g[12][21] = "curve-se"
	g[11][21] = "straight-v"
	for c := 12; c <= 20; c++ {
		g[10][c] = "straight-h"
	}
	g[10][21] = "curve-ne"
	g[10][11] = "chicane"
	g[10][10] = "chicane"
	g[10][8] = "curve-sw"
	for r := 5; r <= 9; r++ {
		g[r][8] = "straight-v"
	}
	g[4][8] = "curve-nw"
	for c := 9; c <= 26; c++ {
		g[4][c] = "straight-h"
	}
	g[4][27] = "curve-se"
	for r := 5; r <= 11; r++ {
		g[r][27] = "straight-v"
	}
	g[12][27] = "curve-ne"
	for c := 22; c <= 26; c++ {
		g[12][c] = "straight-h"
	}
	g[13][10] = "pit-entry"
	g[13][11] = "pit-exit"
	g[7][15] = "grandstand"
	g[7][16] = "grandstand"
	g[2][20] = "tree"
	g[2][21] = "tree"
	return &Track{
		ID: "f1-circuit", Name: "F1 Circuit",
		Width: w, Height: h, Tiles: g,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}
