package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// Indexer watches a directory for markdown file changes and keeps the
// Qdrant index in sync. It is driven by the parent Service.
type Indexer struct {
	svc    *Service
	logger *zap.Logger
}

// NewIndexer creates an Indexer backed by the given Service.
func NewIndexer(svc *Service, logger *zap.Logger) *Indexer {
	return &Indexer{svc: svc, logger: logger}
}

// IndexAll performs a full initial index of all markdown files in KnowledgeDir,
// recursively traversing subdirectories. Failures on individual files are logged
// but do not stop the pass.
func (idx *Indexer) IndexAll(ctx context.Context) {
	dir := idx.svc.KnowledgeDir()
	idx.logger.Info("starting initial knowledge index pass", zap.String("dir", dir))
	indexed := 0
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			idx.logger.Warn("walk error during initial index",
				zap.String("path", path), zap.Error(err))
			return nil
		}
		if d.IsDir() || !isMarkdown(d.Name()) {
			return nil
		}
		if err := idx.svc.IndexFile(ctx, path); err != nil {
			idx.logger.Warn("failed to index file (initial pass)",
				zap.String("file", path), zap.Error(err))
			return nil
		}
		indexed++
		return nil
	}); err != nil {
		idx.logger.Warn("failed to walk knowledge dir — skipping initial index",
			zap.String("dir", dir), zap.Error(err))
		return
	}
	idx.logger.Info("initial knowledge index complete",
		zap.String("dir", dir), zap.Int("files", indexed))
}

// Watch starts an fsnotify watcher on dir and re-indexes files as they change.
// Blocks until ctx is cancelled. Watcher errors are logged and the watch
// restarts after a brief delay to handle transient failures.
func (idx *Indexer) Watch(ctx context.Context, dir string) {
	for {
		if err := idx.runWatcher(ctx, dir); err != nil {
			if ctx.Err() != nil {
				idx.logger.Info("knowledge watcher stopped")
				return
			}
			idx.logger.Warn("knowledge watcher error — restarting in 10s",
				zap.Error(err))
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return
			}
		} else {
			return // normal shutdown
		}
	}
}

func (idx *Indexer) runWatcher(ctx context.Context, dir string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Register all existing directories so subdirectory changes are watched.
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		return watcher.Add(path)
	}); err != nil {
		return err
	}

	idx.logger.Info("knowledge watcher started", zap.String("dir", dir))

	// Debounce: collect events and process after a short quiet period.
	// The select loop serializes access to pending, so no mutex is needed.
	pending := make(map[string]struct{})
	debounce := time.NewTimer(0)
	defer debounce.Stop()
	<-debounce.C // drain initial fire

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// If a directory was created, add it to the watcher.
			if event.Has(fsnotify.Create) {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					_ = watcher.Add(event.Name)
					continue
				}
			}
			name := filepath.Base(event.Name)
			if !isMarkdown(name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				pending[event.Name] = struct{}{}
				debounce.Reset(2 * time.Second)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			idx.logger.Warn("watcher error", zap.Error(err))

		case <-debounce.C:
			if len(pending) == 0 {
				continue
			}
			for path := range pending {
				idx.processEvent(ctx, path)
			}
			pending = make(map[string]struct{})
		}
	}
}

func (idx *Indexer) processEvent(ctx context.Context, path string) {
	filename := filepath.Base(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File removed — delete from Qdrant
		if err := idx.svc.qdrant.DeleteBySourceFile(ctx, filename); err != nil {
			idx.logger.Warn("failed to delete removed file from qdrant",
				zap.String("file", filename), zap.Error(err))
		} else {
			idx.logger.Info("removed deleted file from index", zap.String("file", filename))
		}
		return
	}

	if err := idx.svc.IndexFile(ctx, path); err != nil {
		idx.logger.Warn("failed to re-index changed file",
			zap.String("file", filename), zap.Error(err))
	}
}

func isMarkdown(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}
