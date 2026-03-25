package stage

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const barWidth = 30

type progressBar struct {
	total     int64
	startTime time.Time
	lastBytes int64
	lastTime  time.Time
	mu        sync.Mutex
	out       io.Writer
	done      bool
}

func newProgressBar(total int64) *progressBar {
	now := time.Now()
	return &progressBar{
		total:     total,
		startTime: now,
		lastTime:  now,
		out:       os.Stderr,
	}
}

func (b *progressBar) update(downloaded int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done {
		return
	}

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()

	var speed float64
	if elapsed > 0 {
		speed = float64(downloaded-b.lastBytes) / elapsed
	}
	b.lastBytes = downloaded
	b.lastTime = now

	var pct float64
	var eta string
	if b.total > 0 {
		pct = float64(downloaded) / float64(b.total) * 100
		if speed > 0 {
			remaining := float64(b.total-downloaded) / speed
			eta = fmt.Sprintf(" ETA %s", formatDuration(time.Duration(remaining)*time.Second))
		} else {
			eta = " ETA --"
		}
	} else {
		eta = ""
	}

	filled := 0
	if b.total > 0 && pct > 0 {
		filled = int(pct / 100 * barWidth)
		if filled > barWidth {
			filled = barWidth
		}
	}
	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">"
		bar += strings.Repeat(" ", barWidth-filled-1)
	}

	var line string
	if b.total > 0 {
		line = fmt.Sprintf("\r[%s] %5.1f%% %s/%s %s/s%s",
			bar, pct,
			formatBytes(downloaded), formatBytes(b.total),
			formatBytes(int64(speed)),
			eta,
		)
	} else {
		line = fmt.Sprintf("\r[%-*s] %s %s/s",
			barWidth, bar,
			formatBytes(downloaded),
			formatBytes(int64(speed)),
		)
	}

	fmt.Fprint(b.out, line)
}

func (b *progressBar) finish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done {
		return
	}
	b.done = true
	elapsed := time.Since(b.startTime)
	var avgSpeed float64
	if elapsed.Seconds() > 0 {
		avgSpeed = float64(b.lastBytes) / elapsed.Seconds()
	}
	fmt.Fprintf(b.out, "\r[%s] 100.0%% %s %s/s — done in %s\n",
		strings.Repeat("=", barWidth),
		formatBytes(b.lastBytes),
		formatBytes(int64(avgSpeed)),
		formatDuration(elapsed),
	)
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
