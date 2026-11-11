package runner

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watchedPath struct {
	path      string
	directory bool
}

type fileWatch struct {
	w      *fsnotify.Watcher
	done   chan struct{}
	roots  []watchedPath
	ignore []string
}

func watch(c config, triggers chan<- struct{}, errs chan<- error) (*fileWatch, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	f := &fileWatch{w: w, done: make(chan struct{}), ignore: append([]string{".git", ".hg", ".svn", "node_modules", "target", "dist"}, c.ignore...)}
	fail := func(err error) (*fileWatch, error) { w.Close(); return nil, err }
	for _, name := range c.watch {
		if !filepath.IsAbs(name) {
			name = filepath.Join(c.directory, name)
		}
		name, err = filepath.EvalSymlinks(name)
		if err != nil {
			return fail(err)
		}
		info, err := os.Stat(name)
		if err != nil {
			return fail(err)
		}
		f.roots = append(f.roots, watchedPath{name, info.IsDir()})
		// Watch the parent so atomic saves and replacement of a watched root survive.
		if err := w.Add(filepath.Dir(name)); err != nil {
			return fail(err)
		}
		if info.IsDir() {
			if err := f.addTree(name); err != nil {
				return fail(err)
			}
		}
	}
	go f.loop(c.debounce, triggers, errs)
	return f, nil
}

func (f *fileWatch) Close() { f.w.Close(); <-f.done }

func (f *fileWatch) relevant(name string) bool {
	for _, root := range f.roots {
		if name == root.path {
			return true
		}
		if !root.directory {
			continue
		}
		rel, err := filepath.Rel(root.path, name)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		ignored := false
		for _, pattern := range f.ignore {
			if match, _ := path.Match(pattern, filepath.ToSlash(rel)); match {
				ignored = true
				break
			}
			for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
				if match, _ := path.Match(pattern, part); match {
					ignored = true
					break
				}
			}
			if ignored {
				break
			}
		}
		if !ignored {
			return true
		}
	}
	return false
}

func (f *fileWatch) addTree(root string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if !f.relevant(name) {
			return filepath.SkipDir
		}
		return f.w.Add(name)
	})
}

func (f *fileWatch) loop(debounce time.Duration, triggers chan<- struct{}, errs chan<- error) {
	defer close(f.done)
	var timer *time.Timer
	var tick <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	report := func(err error) {
		select {
		case errs <- err:
		default:
		}
	}
	for {
		select {
		case event, ok := <-f.w.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) || !f.relevant(event.Name) {
				continue
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := f.addTree(event.Name); err != nil {
						report(fmt.Errorf("%s: %w", event.Name, err))
					}
				}
			}
			if timer == nil {
				timer = time.NewTimer(debounce)
			} else {
				timer.Reset(debounce)
			}
			tick = timer.C
		case <-tick:
			request(triggers)
			tick = nil
		case err, ok := <-f.w.Errors:
			if !ok {
				return
			}
			report(err)
		}
	}
}
