package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
)

var launchHistoryMu sync.Mutex

func launchHistoryPath(preferencesPath string) string {
	if preferencesPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(preferencesPath), "launch-history.json")
}

func loadLaunchHistory(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read launch history: %w", err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("decode launch history: %w", err)
	}
	return mergeLaunchPaths(paths), nil
}

// mergeLaunchPaths preserves the first occurrence, with most recent paths first.
func mergeLaunchPaths(groups ...[]string) []string {
	var paths []string
	for _, group := range groups {
		for _, path := range group {
			if !filepath.IsAbs(path) {
				continue
			}
			path = filepath.Clean(path)
			if !slices.Contains(paths, path) {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func saveLaunchPath(path, confirmed string) (err error) {
	if path == "" {
		return nil
	}
	launchHistoryMu.Lock()
	defer launchHistoryMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create launch history directory: %w", err)
	}
	// Match notes.Store: lock a stable sidecar, since atomic rename replaces
	// the history inode. Every process reads the latest data under this lock.
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open launch history lock: %w", err)
	}
	defer func() {
		if closeErr := lock.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close launch history lock: %w", closeErr))
		}
	}()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock launch history: %w", err)
	}
	defer func() {
		if unlockErr := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); unlockErr != nil {
			err = errors.Join(err, fmt.Errorf("unlock launch history: %w", unlockErr))
		}
	}()
	paths, err := loadLaunchHistory(path)
	if err != nil {
		return err
	}
	if len(paths) > 0 && paths[0] == filepath.Clean(confirmed) {
		return nil
	}
	data, err := json.MarshalIndent(mergeLaunchPaths([]string{confirmed}, paths), "", "  ")
	if err != nil {
		return fmt.Errorf("encode launch history: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".launch-history-*")
	if err != nil {
		return fmt.Errorf("create launch history: %w", err)
	}
	defer func() {
		if removeErr := os.Remove(file.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary launch history: %w", removeErr))
		}
	}()
	if _, err := file.Write(data); err != nil {
		closeErr := file.Close()
		return fmt.Errorf("write launch history: %w", errors.Join(err, closeErr))
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close launch history: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace launch history: %w", err)
	}
	return nil
}

func launchDirectory(input, base string) (string, error) {
	if input == "" {
		return "", errors.New("enter a project directory")
	}
	path := input
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) && base != "" {
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("open project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", path)
	}
	return path, nil
}

// rememberSelectedLaunchProject tracks normal project selections across restarts.
// Call after selection changes and after the initial snapshot restores selection.
func (m *Model) rememberSelectedLaunchProject() {
	a := m.selectedAgent()
	if a == nil {
		return
	}
	path := a.CWD
	if path == "" {
		path = a.Project
	}
	path, err := launchDirectory(path, m.currentDir)
	if err != nil {
		return // Stale snapshot directories are not confirmed launch candidates.
	}
	if err := saveLaunchPath(launchHistoryPath(m.preferencesPath), path); err != nil {
		m.notice = err.Error()
	}
}
