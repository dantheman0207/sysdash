package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gdisk "github.com/shirou/gopsutil/v4/disk"

	"github.com/daniel/sysdash/internal/domain"
)

type CleanupRunner struct {
	mu       sync.RWMutex
	progress domain.CleanupProgress
	history  []domain.CleanupRunSummary
}

type cleanupStep struct {
	name string
	run  func(context.Context) error
}

func NewCleanupRunner() *CleanupRunner {
	return &CleanupRunner{}
}

func (r *CleanupRunner) Progress() domain.CleanupProgress {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := r.progress
	out.Results = append([]domain.CleanupStepResult(nil), out.Results...)
	out.History = append([]domain.CleanupRunSummary(nil), r.history...)
	return out
}

func (r *CleanupRunner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.progress.Running {
		r.mu.Unlock()
		return fmt.Errorf("cleanup already running")
	}
	beforeFree := diskFreeBytes()
	r.progress = domain.CleanupProgress{
		Running:    true,
		Completed:  0,
		Total:      5,
		StartedAt:  time.Now(),
		BeforeFree: beforeFree,
		Results:    make([]domain.CleanupStepResult, 0, 5),
		History:    append([]domain.CleanupRunSummary(nil), r.history...),
	}
	r.mu.Unlock()

	go r.run(ctx)
	return nil
}

func (r *CleanupRunner) run(ctx context.Context) {
	steps := []cleanupStep{
		{name: "go clean -cache -modcache", run: func(ctx context.Context) error { return runCommand(ctx, "go", "clean", "-cache", "-modcache") }},
		{name: "remove developer and user caches", run: removeDeveloperCaches},
		{name: "empty trash", run: emptyTrash},
		{name: "brew cleanup -s", run: func(ctx context.Context) error { return runCommand(ctx, "brew", "cleanup", "-s") }},
		{name: "docker system prune -f (best effort)", run: func(ctx context.Context) error {
			_ = runCommand(ctx, "docker", "system", "prune", "-f")
			return nil
		}},
	}

	for i, step := range steps {
		started := time.Now()
		r.setCurrent(step.name, i)
		err := step.run(ctx)
		detail := "ok"
		ok := true
		if err != nil {
			ok = false
			detail = err.Error()
		}
		r.finishStep(domain.CleanupStepResult{
			Name:    step.name,
			OK:      ok,
			Detail:  detail,
			Started: started,
			Ended:   time.Now(),
		})
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress.Running = false
	r.progress.Done = true
	r.progress.CurrentStep = ""
	r.progress.FinishedAt = time.Now()
	r.progress.AfterFree = diskFreeBytes()
	if r.progress.AfterFree > r.progress.BeforeFree {
		r.progress.SavedBytes = r.progress.AfterFree - r.progress.BeforeFree
	}
	run := domain.CleanupRunSummary{
		FinishedAt: r.progress.FinishedAt,
		SavedBytes: r.progress.SavedBytes,
		Failures:   cleanupFailures(r.progress.Results),
	}
	r.history = append([]domain.CleanupRunSummary{run}, r.history...)
	if len(r.history) > 5 {
		r.history = r.history[:5]
	}
	r.progress.History = append([]domain.CleanupRunSummary(nil), r.history...)
}

func (r *CleanupRunner) setCurrent(name string, completed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress.CurrentStep = name
	r.progress.Completed = completed
}

func (r *CleanupRunner) finishStep(result domain.CleanupStepResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress.Results = append(r.progress.Results, result)
	r.progress.Completed = len(r.progress.Results)
}

func diskFreeBytes() uint64 {
	usage, err := gdisk.UsageWithContext(context.Background(), "/")
	if err != nil {
		return 0
	}
	if usage.Total < usage.Used {
		return 0
	}
	return usage.Total - usage.Used
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s", text)
}

func removeDeveloperCaches(context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	paths := []string{
		filepath.Join(home, "Library/Developer/Xcode/DerivedData"),
		filepath.Join(home, "Library/Developer/CoreSimulator/Caches"),
		filepath.Join(home, "Library/Caches"),
		filepath.Join(home, ".cache"),
	}
	var errs []string
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func emptyTrash(context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	usr, err := user.Current()
	if err != nil {
		return err
	}

	paths := []string{filepath.Join(home, ".Trash")}
	volumeRoots, err := os.ReadDir("/Volumes")
	if err == nil {
		for _, entry := range volumeRoots {
			paths = append(paths, filepath.Join("/Volumes", entry.Name(), ".Trashes", usr.Uid))
		}
	}

	var errs []string
	for _, trash := range paths {
		entries, err := os.ReadDir(trash)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Sprintf("%s: %v", trash, err))
			continue
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(trash, entry.Name())); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", trash, entry.Name(), err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func cleanupFailures(results []domain.CleanupStepResult) int {
	var failures int
	for _, result := range results {
		if !result.OK {
			failures++
		}
	}
	return failures
}
