package summary

import (
	"math"
	"sort"
	"time"

	"github.com/termofeed/termofeed-back/internal/api/gen"
)

type Reaction struct {
	Emoji string
	At    time.Time
}

// CalculateWindow computes the summary for a specific window.
func CalculateWindow(
	window gen.SummaryWindow,
	reactions []Reaction,
	palette []string,
	roomCreatedAt time.Time,
	roomEndedAt *time.Time,
	now time.Time,
) gen.Summary {
	var from, to time.Time

	switch window {
	case gen.SummaryWindowN30s:
		to = now
		from = now.Add(-30 * time.Second)
	case gen.SummaryWindowN1m:
		to = now
		from = now.Add(-60 * time.Second)
	case gen.SummaryWindowN5m:
		to = now
		from = now.Add(-300 * time.Second)
	case gen.SummaryWindowAll:
		from = roomCreatedAt
		if roomEndedAt != nil && !roomEndedAt.IsZero() {
			to = *roomEndedAt
		} else {
			to = now
		}
	default:
		to = now
		from = now.Add(-60 * time.Second)
	}

	// Filter reactions in [from, to]
	counts := make(map[string]int)
	total := 0
	for _, r := range reactions {
		if !r.At.Before(from) && !r.At.After(to) {
			counts[r.Emoji]++
			total++
		}
	}

	if total == 0 {
		return gen.Summary{
			Window:         window,
			From:           from,
			To:             to,
			TotalReactions: 0,
			DominantEmoji:  nil,
			Ranking:        make([]gen.EmojiCount, 0),
		}
	}

	// Palette index map for tie-breaking
	paletteIdx := make(map[string]int, len(palette))
	for i, e := range palette {
		paletteIdx[e] = i
	}

	ranking := make([]gen.EmojiCount, 0, len(counts))
	for emoji, count := range counts {
		pct := float32(math.Round((float64(count)/float64(total))*1000.0) / 10.0)
		ranking = append(ranking, gen.EmojiCount{
			Emoji:      emoji,
			Count:      count,
			Percentage: pct,
		})
	}

	// Sort descending by count; if equal, sort by palette order
	sort.Slice(ranking, func(i, j int) bool {
		if ranking[i].Count != ranking[j].Count {
			return ranking[i].Count > ranking[j].Count
		}
		idxI, okI := paletteIdx[ranking[i].Emoji]
		if !okI {
			idxI = 999999
		}
		idxJ, okJ := paletteIdx[ranking[j].Emoji]
		if !okJ {
			idxJ = 999999
		}
		if idxI != idxJ {
			return idxI < idxJ
		}
		return ranking[i].Emoji < ranking[j].Emoji
	})

	dominant := ranking[0].Emoji
	return gen.Summary{
		Window:         window,
		From:           from,
		To:             to,
		TotalReactions: total,
		DominantEmoji:  &dominant,
		Ranking:        ranking,
	}
}

// CalculateRealtimeWindows returns the 30s, 1m, and 5m summaries for summary.updated event.
func CalculateRealtimeWindows(
	reactions []Reaction,
	palette []string,
	roomCreatedAt time.Time,
	now time.Time,
) map[string]gen.Summary {
	windows := make(map[string]gen.Summary, 3)
	windows[string(gen.SummaryWindowN30s)] = CalculateWindow(gen.SummaryWindowN30s, reactions, palette, roomCreatedAt, nil, now)
	windows[string(gen.SummaryWindowN1m)] = CalculateWindow(gen.SummaryWindowN1m, reactions, palette, roomCreatedAt, nil, now)
	windows[string(gen.SummaryWindowN5m)] = CalculateWindow(gen.SummaryWindowN5m, reactions, palette, roomCreatedAt, nil, now)
	return windows
}
