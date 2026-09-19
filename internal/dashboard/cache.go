package dashboard

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type snapshot struct {
	Version    int                `json:"version"`
	Source     string             `json:"source"`
	FetchedAt  time.Time          `json:"fetchedAt"`
	Files      map[string]session `json:"files"`
	Unreadable int                `json:"unreadable"`
}
type localStore struct {
	mu                 sync.Mutex
	current            *snapshot // Immutable once published; readers never wait for scanning.
	pending            chan struct{}
	lastError          string
	root, path, source string
	ctx                context.Context
	wg                 sync.WaitGroup
	public             *publicSnapshot // Configured before run; only successful scans publish.
}

func newLocalStore(ctx context.Context, root, state string) (*localStore, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	s := &localStore{root: root, path: filepath.Join(state, "sessions.json"), source: fmt.Sprintf("%x", sha256.Sum256([]byte(root))), ctx: ctx}
	var saved snapshot
	if err := readJSON(s.path, &saved); err == nil {
		if saved.Version == 3 && saved.Source == s.source && !saved.FetchedAt.IsZero() && saved.Files != nil {
			s.current = &saved
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		s.lastError = "缓存无法读取，正在重新统计"
		log.Print("本机缓存无法读取，将重新扫描")
	}
	return s, nil
}
func (s *localStore) view() (*snapshot, bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current, s.pending != nil, s.lastError
}
func (s *localStore) refresh() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending != nil {
		return s.pending
	}
	done := make(chan struct{})
	s.pending = done
	old := s.current
	s.wg.Go(func() {
		next, err := s.scan(old)
		if err == nil {
			err = atomicJSON(s.path, next)
		}
		if err == nil && s.public != nil {
			if publishErr := s.public.publish(next); publishErr != nil {
				log.Printf("公开快照更新失败，保留上次内容: %v", publishErr)
			}
		}
		s.mu.Lock()
		if err != nil {
			s.lastError = "本机缓存更新失败，保留上次数据"
			if old == nil {
				s.lastError = "本机数据分析失败，请重试"
			}
			if !errors.Is(err, context.Canceled) {
				log.Printf("本机缓存更新失败: %v", err)
			}
		} else {
			s.current = next
			s.lastError = ""
		}
		s.pending = nil
		close(done)
		s.mu.Unlock()
	})
	return done
}
func (s *localStore) run(interval time.Duration) {
	s.wg.Go(func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(interval):
				s.refresh()
			}
		}
	})
	// Warm missing/stale caches without delaying the HTTP server.
	data, _, _ := s.view()
	if data == nil || time.Since(data.FetchedAt) >= interval {
		s.refresh()
	}
}
func (s *localStore) scan(old *snapshot) (*snapshot, error) {
	next := &snapshot{Version: 3, Source: s.source, Files: map[string]session{}}
	for _, dir := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(s.root, dir)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if err := s.ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				if path == root && errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") || !d.Type().IsRegular() {
				return nil
			}
			relative, err := filepath.Rel(s.root, path)
			if err != nil {
				return err
			}
			var previous session
			var exists bool
			if old != nil {
				previous, exists = old.Files[relative]
			}
			info, err := d.Info()
			if err == nil && exists && previous.Size == info.Size() && previous.Modified == info.ModTime().UnixNano() {
				next.Files[relative] = previous
				return nil
			}
			var parsed session
			if err == nil {
				parsed, err = parseSession(s.ctx, path)
			}
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				next.Unreadable++
				if exists {
					next.Files[relative] = previous
				}
				return nil
			}
			parsed.Size = info.Size()
			parsed.Modified = info.ModTime().UnixNano()
			next.Files[relative] = parsed
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	next.FetchedAt = time.Now()
	return next, nil
}
func atomicJSON(path string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
