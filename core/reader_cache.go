package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvshp/ReadCLI/reader"
)

func cachedReaderForPath(path string) (reader.Reader, bool, error) {
	path = normalizeBookPath(path)
	if r, cached, err := cachedReaderIfFresh(path); cached || err != nil {
		return r, cached, err
	}

	r, size, modTime, err := loadFreshReader(path)
	if err != nil {
		return nil, false, err
	}
	storeCachedReader(path, r, size, modTime)
	return r, false, nil
}

func cachedReaderIfFresh(path string) (reader.Reader, bool, error) {
	path = normalizeBookPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	if app != nil && app.readerCache != nil {
		if cached, ok := app.readerCache[path]; ok && cached.reader != nil {
			if cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
				return cached.reader, true, nil
			}
			delete(app.readerCache, path)
		}
	}

	return nil, false, nil
}

func loadFreshReader(path string) (reader.Reader, int64, time.Time, error) {
	path = normalizeBookPath(path)
	r, err := newReaderForPath(path)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	if err := r.Load(path); err != nil {
		return nil, 0, time.Time{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	return r, info.Size(), info.ModTime(), nil
}

func storeCachedReader(path string, r reader.Reader, size int64, modTime time.Time) {
	path = normalizeBookPath(path)
	if app != nil {
		if app.readerCache == nil {
			app.readerCache = map[string]cachedReader{}
		}
		app.readerCache[path] = cachedReader{reader: r, size: size, modTime: modTime}
	}
}

func normalizeBookPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func newReaderForPath(path string) (reader.Reader, error) {
	switch strings.ToUpper(filepath.Ext(path)) {
	case ".TXT":
		return reader.NewTxtReader(), nil
	case ".EPUB":
		return reader.NewEpubReader(), nil
	default:
		return nil, fmt.Errorf("unsupported file format: %s", filepath.Ext(path))
	}
}
