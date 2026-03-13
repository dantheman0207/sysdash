package diskscan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/daniel/sysdash/internal/domain"
)

type Scanner struct {
	mu       sync.RWMutex
	cache    map[string]domain.DiskNode
	inflight map[string]bool
}

func New() *Scanner {
	return &Scanner{
		cache:    make(map[string]domain.DiskNode),
		inflight: make(map[string]bool),
	}
}

func (s *Scanner) Cached(path string) (domain.DiskNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.cache[path]
	return node, ok
}

func (s *Scanner) IsInflight(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inflight[path]
}

func (s *Scanner) Scan(ctx context.Context, path string) (domain.DiskNode, error) {
	s.mu.Lock()
	if node, ok := s.cache[path]; ok {
		s.mu.Unlock()
		return node, nil
	}
	if s.inflight[path] {
		s.mu.Unlock()
		return domain.DiskNode{Path: path}, nil
	}
	s.inflight[path] = true
	s.mu.Unlock()

	node, err := scanPath(ctx, path, deviceID(path))

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, path)
	if err == nil {
		s.cache[path] = node
	}
	return node, err
}

func scanPath(ctx context.Context, path string, rootDev uint64) (domain.DiskNode, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return domain.DiskNode{}, err
	}

	node := domain.DiskNode{Path: path, ScannedAt: time.Now()}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return domain.DiskNode{}, ctx.Err()
		default:
		}

		full := filepath.Join(path, entry.Name())
		size, incomplete := dirSize(ctx, full, rootDev)
		child := domain.DiskEntry{
			Name:       entry.Name(),
			Path:       full,
			Size:       size,
			IsDir:      entry.IsDir(),
			ScannedAt:  node.ScannedAt,
			Incomplete: incomplete,
		}
		node.Children = append(node.Children, child)
		node.Size += size
	}

	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].Size == node.Children[j].Size {
			return node.Children[i].Name < node.Children[j].Name
		}
		return node.Children[i].Size > node.Children[j].Size
	})

	if node.Size > 0 {
		for i := range node.Children {
			node.Children[i].Percent = (float64(node.Children[i].Size) / float64(node.Size)) * 100
		}
	}

	return node, nil
}

func dirSize(ctx context.Context, path string, rootDev uint64) (uint64, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, true
	}
	if !info.IsDir() {
		return uint64(info.Size()), false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, true
	}

	if dev := statDevice(info); rootDev != 0 && dev != 0 && dev != rootDev {
		return 0, true
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, true
	}

	var total uint64
	var incomplete bool
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return total, true
		default:
		}

		child := filepath.Join(path, entry.Name())
		size, childIncomplete := dirSize(ctx, child, rootDev)
		total += size
		incomplete = incomplete || childIncomplete
	}
	return total, incomplete
}

func deviceID(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return statDevice(info)
}

func statDevice(info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(stat.Dev)
}

func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
