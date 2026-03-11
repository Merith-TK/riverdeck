package imaging

import (
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// GIFData holds all decoded frames and their inter-frame delays.
type GIFData struct {
	Frames []image.Image
	Delays []int // centiseconds per frame (as returned by image/gif)
}

// gifCache caches decoded GIF animations by path.
type gifCache struct {
	mu      sync.RWMutex
	entries map[string]*GIFData
}

var globalGIFCache = &gifCache{entries: make(map[string]*GIFData)}

func (c *gifCache) get(key string) (*GIFData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.entries[key]
	return d, ok
}

func (c *gifCache) set(key string, d *GIFData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = d
}

// LoadGIFFrames decodes all frames of an animated GIF file or URL.
// Returns a GIFData containing every frame as an image.Image and the
// per-frame delay in centiseconds (multiply by 10 for milliseconds).
// Results are cached so repeated calls for the same path are free.
func LoadGIFFrames(path string) (*GIFData, error) {
	if d, ok := globalGIFCache.get(path); ok {
		return d, nil
	}

	var reader io.ReadCloser
	var err error

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch GIF: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d fetching GIF", resp.StatusCode)
		}
		reader = resp.Body
	} else {
		reader, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open GIF: %w", err)
		}
	}
	defer reader.Close()

	g, err := gif.DecodeAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode GIF: %w", err)
	}

	// Composite each frame onto the canvas of the previous one so that
	// partial/overlay frames render correctly.
	bounds := g.Image[0].Bounds()
	canvas := image.NewRGBA(bounds)
	frames := make([]image.Image, len(g.Image))

	for i, frame := range g.Image {
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		// Deep-copy the canvas so each frame is an independent snapshot.
		snap := image.NewRGBA(canvas.Bounds())
		copy(snap.Pix, canvas.Pix)
		frames[i] = snap

		// disposal == 2: restore to background (clear to transparent)
		if disposal == 2 {
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		}
	}

	data := &GIFData{Frames: frames, Delays: g.Delay}
	globalGIFCache.set(path, data)
	return data, nil
}
