// Package project provides utilities for finding and working with the project root directory.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrGoModNotFound indicates that go.mod could not be found when searching for project root.
var ErrGoModNotFound = errors.New("go.mod not found")

var (
	rootMu  sync.RWMutex
	root    string
	rootErr error
)

// tryWorkDir attempts to use WORK_DIR environment variable as project root.
// Returns (root, nil) if valid, (nil, error) if invalid and should fail,
// or (nil, nil) if should fall through to searching.
func tryWorkDir() (string, error) {
	envDir := os.Getenv("WORK_DIR")
	if envDir == "" {
		return "", nil
	}

	absDir, err := filepath.Abs(envDir)
	if err != nil {
		// filepath.Abs rarely fails, but if it does, fall through to searching
		// This is intentional: we want to search from current dir instead
		return "", nil //nolint:nilerr // Intentionally ignore abs path error to fall through
	}

	absDir = filepath.Clean(absDir)
	info, statErr := os.Stat(absDir)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", nil // Fall through to searching
		}
		// Some other error like permission error - return it
		return "", fmt.Errorf("invalid WORK_DIR %q: %w", envDir, statErr)
	}

	if !info.IsDir() {
		return "", nil // Fall through to searching
	}

	// If go.mod exists under WORK_DIR, accept it
	if _, gmErr := os.Stat(filepath.Join(absDir, "go.mod")); gmErr == nil {
		return absDir, nil
	}

	// If there is no go.mod, fall through to searching
	return "", nil
}

// searchUpward searches for go.mod by walking up from the given directory.
func searchUpward(startDir string) (string, error) {
	startDir, _ = filepath.Abs(startDir)
	startDir = filepath.Clean(startDir)

	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if info, statErr := os.Stat(goModPath); statErr == nil && !info.IsDir() {
			return dir, nil
		} else if statErr != nil && !os.IsNotExist(statErr) {
			// Unexpected error reading go.mod (permission, IO) - return it
			return "", fmt.Errorf("error accessing %s: %w", goModPath, statErr)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached file system root
			return "", fmt.Errorf("%w (started from: %s)", ErrGoModNotFound, startDir)
		}
		dir = parent
	}
}

// Root returns the project root directory path.
// Priority:
//  1. WORK_DIR environment variable (if set and valid)
//  2. Search upward from current working directory for go.mod
//
// Result is cached for performance. Use Reset() to clear the cache.
func Root() (string, error) {
	// Fast read path
	rootMu.RLock()
	if root != "" || rootErr != nil {
		dir, err := root, rootErr
		rootMu.RUnlock()

		return dir, err
	}
	rootMu.RUnlock()

	// Slow write path
	rootMu.Lock()
	defer rootMu.Unlock()
	// Double check
	if root != "" || rootErr != nil {
		return root, rootErr
	}

	// 1) Try WORK_DIR first
	if workDir, err := tryWorkDir(); err != nil {
		rootErr = err

		return "", rootErr
	} else if workDir != "" {
		root = workDir

		return root, nil
	}

	// 2) Search upward from current working directory
	startDir, err := os.Getwd()
	if err != nil {
		rootErr = fmt.Errorf("failed to get working directory: %w", err)

		return "", rootErr
	}

	foundRoot, err := searchUpward(startDir)
	if err != nil {
		rootErr = err

		return "", rootErr
	}

	root = foundRoot

	return root, nil
}

// MustRoot returns the root directory path, panicking if it cannot be determined.
func MustRoot() string {
	dir, err := Root()
	if err != nil {
		panic(fmt.Sprintf("failed to find project root: %v", err))
	}

	return dir
}

// Reset clears the cached project root. Useful for tests or when WORK_DIR changes.
func Reset() {
	rootMu.Lock()
	defer rootMu.Unlock()
	root = ""
	rootErr = nil
}

// Path returns the absolute path joined with project root and the provided relative path.
// It does not check existence by default (optionally you can check if file exists).
func Path(rel string) (string, error) {
	r, err := Root()
	if err != nil {
		return "", err
	}

	return filepath.Join(r, rel), nil
}
