package summary_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/summary"
)

func TestCalculateWindow_Empty(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	palette := []string{"😀", "😂", "😍"}

	s := summary.CalculateWindow(gen.SummaryWindowN1m, nil, palette, now.Add(-10*time.Minute), nil, now)

	assert.Equal(t, gen.SummaryWindowN1m, s.Window)
	assert.Equal(t, 0, s.TotalReactions)
	assert.Nil(t, s.DominantEmoji)
	assert.Empty(t, s.Ranking)
	assert.Equal(t, now.Add(-60*time.Second), s.From)
	assert.Equal(t, now, s.To)
}

func TestCalculateWindow_RankingAndPercentages(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	palette := []string{"😀", "😂", "😍", "👏", "😮"}

	// 12 😂, 8 👏, 4 😮
	var reactions []summary.Reaction
	for i := 0; i < 12; i++ {
		reactions = append(reactions, summary.Reaction{Emoji: "😂", At: now.Add(-10 * time.Second)})
	}
	for i := 0; i < 8; i++ {
		reactions = append(reactions, summary.Reaction{Emoji: "👏", At: now.Add(-20 * time.Second)})
	}
	for i := 0; i < 4; i++ {
		reactions = append(reactions, summary.Reaction{Emoji: "😮", At: now.Add(-30 * time.Second)})
	}

	s := summary.CalculateWindow(gen.SummaryWindowN1m, reactions, palette, now.Add(-10*time.Minute), nil, now)

	assert.Equal(t, 24, s.TotalReactions)
	assert.NotNil(t, s.DominantEmoji)
	assert.Equal(t, "😂", *s.DominantEmoji)
	assert.Len(t, s.Ranking, 3)

	assert.Equal(t, "😂", s.Ranking[0].Emoji)
	assert.Equal(t, 12, s.Ranking[0].Count)
	assert.InDelta(t, 50.0, s.Ranking[0].Percentage, 0.1)

	assert.Equal(t, "👏", s.Ranking[1].Emoji)
	assert.Equal(t, 8, s.Ranking[1].Count)
	assert.InDelta(t, 33.3, s.Ranking[1].Percentage, 0.1)

	assert.Equal(t, "😮", s.Ranking[2].Emoji)
	assert.Equal(t, 4, s.Ranking[2].Count)
	assert.InDelta(t, 16.7, s.Ranking[2].Percentage, 0.1)
}

func TestCalculateWindow_TieBreakerByPalette(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	// Palette order: 😍 comes before 😂
	palette := []string{"😍", "😂", "👏"}

	reactions := []summary.Reaction{
		{Emoji: "😂", At: now.Add(-5 * time.Second)},
		{Emoji: "😍", At: now.Add(-5 * time.Second)},
	}

	s := summary.CalculateWindow(gen.SummaryWindowN30s, reactions, palette, now.Add(-10*time.Minute), nil, now)

	assert.Equal(t, 2, s.TotalReactions)
	assert.NotNil(t, s.DominantEmoji)
	assert.Equal(t, "😍", *s.DominantEmoji)
	assert.Equal(t, "😍", s.Ranking[0].Emoji)
	assert.Equal(t, "😂", s.Ranking[1].Emoji)
}

func TestCalculateWindow_FilterByWindow(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	palette := []string{"😀", "😂"}

	reactions := []summary.Reaction{
		{Emoji: "😀", At: now.Add(-10 * time.Second)},  // inside 30s
		{Emoji: "😂", At: now.Add(-45 * time.Second)},  // outside 30s, inside 1m
		{Emoji: "😀", At: now.Add(-120 * time.Second)}, // outside 1m, inside 5m
	}

	s30s := summary.CalculateWindow(gen.SummaryWindowN30s, reactions, palette, now.Add(-10*time.Minute), nil, now)
	assert.Equal(t, 1, s30s.TotalReactions)
	assert.Equal(t, "😀", *s30s.DominantEmoji)

	s1m := summary.CalculateWindow(gen.SummaryWindowN1m, reactions, palette, now.Add(-10*time.Minute), nil, now)
	assert.Equal(t, 2, s1m.TotalReactions)

	s5m := summary.CalculateWindow(gen.SummaryWindowN5m, reactions, palette, now.Add(-10*time.Minute), nil, now)
	assert.Equal(t, 3, s5m.TotalReactions)

	sAll := summary.CalculateWindow(gen.SummaryWindowAll, reactions, palette, now.Add(-10*time.Minute), nil, now)
	assert.Equal(t, 3, sAll.TotalReactions)
}

func TestCalculateRealtimeWindows(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	palette := []string{"😀", "😂"}

	reactions := []summary.Reaction{
		{Emoji: "😀", At: now.Add(-10 * time.Second)},
	}

	windows := summary.CalculateRealtimeWindows(reactions, palette, now.Add(-10*time.Minute), now)
	assert.Len(t, windows, 3)
	assert.Contains(t, windows, "30s")
	assert.Contains(t, windows, "1m")
	assert.Contains(t, windows, "5m")
	assert.Equal(t, 1, windows["30s"].TotalReactions)
}
