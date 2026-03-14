package diskscan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/daniel/sysdash/internal/domain"
)

type Scanner struct {
	mu       sync.RWMutex
	cache    map[string]domain.DiskNode
	inflight map[string]bool
	progress map[string]domain.DiskScanProgress
}

type fileID struct {
	dev uint64
	ino uint64
}

type scanState struct {
	rootDev uint64
	root    string
	seen    map[fileID]struct{}
}

func New() *Scanner {
	return &Scanner{
		cache:    make(map[string]domain.DiskNode),
		inflight: make(map[string]bool),
		progress: make(map[string]domain.DiskScanProgress),
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

func (s *Scanner) Progress(path string) (domain.DiskScanProgress, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.progress[path]
	return p, ok
}

func (s *Scanner) Scan(ctx context.Context, path string) (domain.DiskNode, error) {
	return s.scan(ctx, path, false)
}

func (s *Scanner) ForceScan(ctx context.Context, path string) (domain.DiskNode, error) {
	return s.scan(ctx, path, true)
}

func (s *Scanner) scan(ctx context.Context, path string, force bool) (domain.DiskNode, error) {
	s.mu.Lock()
	if !force {
		if node, ok := s.cache[path]; ok {
			s.mu.Unlock()
			return node, nil
		}
	}
	if s.inflight[path] {
		s.mu.Unlock()
		return domain.DiskNode{Path: path}, nil
	}
	if force {
		s.clearCacheUnder(path)
	}
	s.inflight[path] = true
	s.progress[path] = domain.DiskScanProgress{
		Root:      path,
		StartedAt: time.Now(),
		Total:     1,
	}
	s.mu.Unlock()

	rootInfo, err := os.Lstat(path)
	if err != nil {
		s.mu.Lock()
		delete(s.inflight, path)
		delete(s.progress, path)
		s.mu.Unlock()
		return domain.DiskNode{}, err
	}

	state := scanState{
		rootDev: statDevice(rootInfo),
		root:    path,
		seen:    make(map[fileID]struct{}),
	}
	if id, ok := statFileID(rootInfo); ok {
		state.seen[id] = struct{}{}
	}

	node, _, err := s.scanRecursive(ctx, path, path, state)

	s.mu.Lock()
	delete(s.inflight, path)
	if p, ok := s.progress[path]; ok {
		p.Done = true
		if p.Total < p.Completed {
			p.Total = p.Completed
		}
		s.progress[path] = p
	}
	s.mu.Unlock()

	if err != nil {
		return domain.DiskNode{}, err
	}
	return node, nil
}

func (s *Scanner) scanRecursive(ctx context.Context, root, path string, state scanState) (domain.DiskNode, bool, error) {
	select {
	case <-ctx.Done():
		return domain.DiskNode{}, true, ctx.Err()
	default:
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return domain.DiskNode{}, true, err
	}

	node := domain.DiskNode{
		Path:      path,
		ScannedAt: time.Now(),
		Children:  make([]domain.DiskEntry, 0, len(entries)),
	}
	var incomplete bool
	s.addProgressTotal(root, len(entries))

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return domain.DiskNode{}, true, ctx.Err()
		default:
		}

		full := filepath.Join(path, entry.Name())
		s.setProgressCurrent(root, full)
		child, childIncomplete, err := s.scanEntry(ctx, root, full, state)
		if err != nil && !IsCanceled(err) {
			childIncomplete = true
		}
		node.Children = append(node.Children, domain.DiskEntry{
			Name:       entry.Name(),
			Path:       full,
			Size:       child.Size,
			IsDir:      child.IsDir,
			Percent:    0,
			ScannedAt:  node.ScannedAt,
			Incomplete: childIncomplete,
		})
		node.Size += child.Size
		incomplete = incomplete || childIncomplete
		s.incrementProgress(root)
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

	s.mu.Lock()
	s.cache[path] = node
	s.mu.Unlock()

	return node, incomplete, nil
}

type scanResult struct {
	Size  uint64
	IsDir bool
}

func (s *Scanner) scanEntry(ctx context.Context, root, path string, state scanState) (scanResult, bool, error) {
	if shouldSkipPath(state.root, path) {
		return scanResult{Size: 0, IsDir: true}, true, nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return scanResult{}, true, err
	}

	// Never follow symlinks; they are cheap to show but unsafe to recurse.
	if info.Mode()&os.ModeSymlink != 0 {
		return scanResult{Size: 0, IsDir: false}, true, nil
	}

	if dev := statDevice(info); state.rootDev != 0 && dev != 0 && dev != state.rootDev {
		return scanResult{Size: 0, IsDir: info.IsDir()}, true, nil
	}

	if id, ok := statFileID(info); ok {
		if _, seen := state.seen[id]; seen {
			return scanResult{Size: 0, IsDir: info.IsDir()}, true, nil
		}
		state.seen[id] = struct{}{}
	}

	if info.IsDir() && isDataless(info) {
		return scanResult{Size: 0, IsDir: true}, false, nil
	}

	if !info.IsDir() {
		return scanResult{Size: localSize(info), IsDir: false}, false, nil
	}

	node, incomplete, err := s.scanRecursive(ctx, root, path, state)
	if err != nil {
		return scanResult{}, true, err
	}
	return scanResult{Size: node.Size, IsDir: true}, incomplete, nil
}

func (s *Scanner) clearCacheUnder(root string) {
	for path := range s.cache {
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			delete(s.cache, path)
		}
	}
}

func (s *Scanner) addProgressTotal(root string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[root]
	p.Total += n
	s.progress[root] = p
}

func (s *Scanner) setProgressCurrent(root, current string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[root]
	p.Current = current
	s.progress[root] = p
}

func (s *Scanner) incrementProgress(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[root]
	p.Completed++
	s.progress[root] = p
}

func statDevice(info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(stat.Dev)
}

func statFileID(info os.FileInfo) (fileID, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, false
	}
	return fileID{dev: uint64(stat.Dev), ino: uint64(stat.Ino)}, true
}

func statBlocksSize(info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return uint64(info.Size())
	}
	// st_blocks is reported in 512-byte units on Darwin.
	if stat.Blocks > 0 {
		return uint64(stat.Blocks) * 512
	}
	return uint64(info.Size())
}

func shouldSkipPath(root, path string) bool {
	if root != "/" {
		return false
	}
	switch path {
	case "/System/Volumes/Data":
		return true
	}
	return strings.HasPrefix(path, "/System/Volumes/Data/")
}

func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
