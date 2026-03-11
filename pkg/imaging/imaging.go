// Package imaging provides image loading, caching, and manipulation utilities
// for the Riverdeck application. It is intentionally decoupled from the
// scripting and streamdeck packages so that image handling can be reused
// independently.
package imaging

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
	mu      sync.Mutex
	images  map[string]cacheEntry
	maxSize int // MB
}

type cacheEntry struct {
	image    image.Image
	accessed time.Time
	size     int // rough memory size estimate in bytes
}

// NewImageCache creates a new image cache with the given max size in MB.
func NewImageCache(maxSize int) *ImageCache {
	return &ImageCache{
		images:  make(map[string]cacheEntry),
		maxSize: maxSize,
	}
}

// Get retrieves an image from cache.
func (c *ImageCache) Get(key string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

	// Estimate memory size (rough: 4 bytes per pixel for RGBA)
	bounds := img.Bounds()
	size := bounds.Dx() * bounds.Dy() * 4

	entry := cacheEntry{
		image:    img,
		accessed: time.Now(),
		size:     size,
	}

	// Compute current cache size
	totalSize := 0
	for _, e := range c.images {
		totalSize += e.size
	}

	// Evict oldest entries until there is room
	maxBytes := c.maxSize * 1024 * 1024
	for totalSize+size > maxBytes {
		if len(c.images) == 0 {
			break
		}

		var oldestKey string
		var oldestTime time.Time
		for k, e := range c.images {
			if oldestKey == "" || e.accessed.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.accessed
			}
		}

		if oldestKey != "" {
			totalSize -= c.images[oldestKey].size
			delete(c.images, oldestKey)
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

// Global image cache (100 MB default)
var globalImageCache = NewImageCache(100)

// LoadImage loads an image from a file path or URL.
// Supports PNG, JPEG, and GIF formats. Uses caching for repeated loads.
func LoadImage(path string) (image.Image, error) {
	if img, ok := globalImageCache.Get(path); ok {
		return img, nil
	}

	var reader io.ReadCloser
	var err error

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
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
		reader, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open image: %w", err)
		}
	}
	defer reader.Close()

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
		img, _, err = image.Decode(reader)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	globalImageCache.Set(path, img)
	return img, nil
}

// ClearImageCache clears the global image cache.
func ClearImageCache() {
	globalImageCache.Clear()
}
