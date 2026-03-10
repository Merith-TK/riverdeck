package scripting

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ImageCache caches loaded images to avoid repeated disk/network reads.
type ImageCache struct {
	mu      sync.RWMutex
	images  map[string]cacheEntry
	maxSize int
}

type cacheEntry struct {
	image    image.Image
	accessed time.Time
	size     int // rough memory size estimate
}

// NewImageCache creates a new image cache.
func NewImageCache(maxSize int) *ImageCache {
	return &ImageCache{
		images:  make(map[string]cacheEntry),
		maxSize: maxSize,
	}
}

// Get retrieves an image from cache.
func (c *ImageCache) Get(key string) (image.Image, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.images[key]
	if ok {
		entry.accessed = time.Now()
		c.images[key] = entry
		return entry.image, true
	}
	return nil, false
}

// Set stores an image in cache with LRU eviction.
func (c *ImageCache) Set(key string, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Estimate memory size (rough calculation)
	bounds := img.Bounds()
	size := bounds.Dx() * bounds.Dy() * 4 // 4 bytes per pixel (RGBA)

	entry := cacheEntry{
		image:    img,
		accessed: time.Now(),
		size:     size,
	}

	// Check if we need to evict
	totalSize := 0
	for _, e := range c.images {
		totalSize += e.size
	}

	// If adding this image would exceed cache size, evict oldest
	for totalSize+size > c.maxSize*1024*1024 { // maxSize is in MB
		if len(c.images) == 0 {
			break
		}

		// Find oldest entry
		var oldestKey string
		var oldestTime time.Time
		for k, e := range c.images {
			if oldestKey == "" || e.accessed.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.accessed
			}
		}

		if oldestKey != "" {
			delete(c.images, oldestKey)
			totalSize -= c.images[oldestKey].size
		}
	}

	c.images[key] = entry
}

// Clear empties the cache.
func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.images = make(map[string]cacheEntry)
}

// Global image cache
var globalImageCache = NewImageCache(100)

// LoadImage loads an image from a file path or URL.
// Supports PNG, JPEG, and GIF formats.
// Uses caching for repeated loads.
func LoadImage(path string) (image.Image, error) {
	// Check cache first
	if img, ok := globalImageCache.Get(path); ok {
		return img, nil
	}

	var reader io.ReadCloser
	var err error

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		// Fetch from URL
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch image: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d fetching image", resp.StatusCode)
		}
		reader = resp.Body
	} else {
		// Load from file
		reader, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open image: %w", err)
		}
	}
	defer reader.Close()

	// Decode based on extension or content
	ext := strings.ToLower(filepath.Ext(path))
	var img image.Image

	switch ext {
	case ".png":
		img, err = png.Decode(reader)
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(reader)
	case ".gif":
		img, err = gif.Decode(reader)
	default:
		// Try to decode as any supported format
		img, _, err = image.Decode(reader)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Cache it
	globalImageCache.Set(path, img)

	return img, nil
}

// ClearImageCache clears the global image cache.
func ClearImageCache() {
	globalImageCache.Clear()
}
