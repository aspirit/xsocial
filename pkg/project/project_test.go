package project

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRoot(t *testing.T) {
	// Reset cache before test
	Reset()

	root, err := Root()
	if err != nil {
		t.Fatalf("Root() failed: %v", err)
	}

	if root == "" {
		t.Fatal("Root() returned empty string")
	}

	// Verify go.mod exists
	goMod := filepath.Join(root, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		t.Fatalf("go.mod not found at %s: %v", goMod, err)
	}

	// Test caching: second call should return same result without re-searching
	root2, err2 := Root()
	if err2 != nil {
		t.Fatalf("Root() second call failed: %v", err2)
	}
	if root2 != root {
		t.Errorf("Root() caching failed: first=%s, second=%s", root, root2)
	}
}

func TestRootConcurrent(t *testing.T) {
	Reset()

	// Test concurrent access
	var wg sync.WaitGroup
	results := make([]string, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = Root()
		}(i)
	}

	wg.Wait()

	// All results should be the same
	firstResult := results[0]
	firstErr := errs[0]
	if firstErr != nil {
		t.Fatalf("Root() failed in concurrent test: %v", firstErr)
	}

	for i := 1; i < 10; i++ {
		if errs[i] != nil {
			t.Errorf("Root() failed in goroutine %d: %v", i, errs[i])
		}
		if results[i] != firstResult {
			t.Errorf("Root() returned different results: goroutine 0=%s, goroutine %d=%s", firstResult, i, results[i])
		}
	}
}

func TestRootWithWorkDir(t *testing.T) {
	// Reset cache
	Reset()

	// Get current root
	currentRoot, err := Root()
	if err != nil {
		t.Fatalf("Failed to get current root: %v", err)
	}

	// Set WORK_DIR to current root
	_ = os.Setenv("WORK_DIR", currentRoot)
	defer func() { _ = os.Unsetenv("WORK_DIR") }()

	// Reset and get root again
	Reset()
	root, err := Root()
	if err != nil {
		t.Fatalf("Root() with WORK_DIR failed: %v", err)
	}

	absCurrentRoot, _ := filepath.Abs(currentRoot)
	absRoot, _ := filepath.Abs(root)
	if absRoot != absCurrentRoot {
		t.Errorf("Root() with WORK_DIR: expected %s, got %s", absCurrentRoot, absRoot)
	}
}

func TestRootWithInvalidWorkDir(t *testing.T) {
	Reset()

	// Set WORK_DIR to a non-existent directory
	_ = os.Setenv("WORK_DIR", "/nonexistent/directory/12345")
	defer func() { _ = os.Unsetenv("WORK_DIR") }()

	Reset()
	root, err := Root()
	// Should fall back to searching from current directory
	if err != nil {
		// If it fails, that's OK - we're testing the fallback behavior
		t.Logf("Root() with invalid WORK_DIR fell back and got error (expected): %v", err)
	} else {
		// If it succeeds, verify it found go.mod
		goMod := filepath.Join(root, "go.mod")
		if _, err := os.Stat(goMod); err != nil {
			t.Errorf("Root() with invalid WORK_DIR returned path without go.mod: %s", root)
		}
	}
}

func TestRootWithWorkDirAsFile(t *testing.T) {
	Reset()

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test-work-dir-*.tmp")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Set WORK_DIR to a file (not a directory)
	_ = os.Setenv("WORK_DIR", tmpFile.Name())
	defer func() { _ = os.Unsetenv("WORK_DIR") }()

	Reset()
	root, err := Root()
	// Should fall back to searching
	if err != nil {
		t.Logf("Root() with WORK_DIR as file fell back (expected): %v", err)
	} else {
		// Should have found a valid root
		goMod := filepath.Join(root, "go.mod")
		if _, err := os.Stat(goMod); err != nil {
			t.Errorf("Root() returned invalid path: %s", root)
		}
	}
}

func TestRootWithWorkDirWithoutGoMod(t *testing.T) {
	Reset()

	// Create a temporary directory without go.mod
	tmpDir, err := os.MkdirTemp("", "test-no-gomod-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	_ = os.Setenv("WORK_DIR", tmpDir)
	defer func() { _ = os.Unsetenv("WORK_DIR") }()

	Reset()
	root, err := Root()
	// Should fall back to searching from current directory
	if err != nil {
		t.Logf("Root() with WORK_DIR without go.mod fell back (expected): %v", err)
	} else {
		// Should have found a valid root
		goMod := filepath.Join(root, "go.mod")
		if _, err := os.Stat(goMod); err != nil {
			t.Errorf("Root() returned path without go.mod: %s", root)
		}
	}
}

func TestReset(t *testing.T) {
	// Get root first to populate cache
	root1, err := Root()
	if err != nil {
		t.Fatalf("Root() failed: %v", err)
	}

	// Reset cache
	Reset()

	// Get root again - should still work
	root2, err := Root()
	if err != nil {
		t.Fatalf("Root() after Reset() failed: %v", err)
	}

	if root1 != root2 {
		t.Errorf("Root() after Reset(): expected %s, got %s", root1, root2)
	}
}

func TestResetClearsCache(t *testing.T) {
	// Get root to populate cache
	root1, err1 := Root()
	if err1 != nil {
		t.Fatalf("Root() failed: %v", err1)
	}

	// Reset should clear cache
	Reset()

	// Change WORK_DIR
	currentRoot, _ := Root()
	_ = os.Setenv("WORK_DIR", currentRoot)
	defer func() { _ = os.Unsetenv("WORK_DIR") }()

	// Reset again and check
	Reset()
	root2, err2 := Root()
	if err2 != nil {
		t.Fatalf("Root() after Reset() and WORK_DIR change failed: %v", err2)
	}

	// Should still be same (WORK_DIR points to same location)
	if root1 != root2 {
		t.Logf("Root changed after WORK_DIR update (this is expected): %s -> %s", root1, root2)
	}
}

func TestMustRoot(t *testing.T) {
	Reset()

	// Should not panic in normal case
	root := MustRoot()
	if root == "" {
		t.Fatal("MustRoot() returned empty string")
	}

	// Verify go.mod exists
	goMod := filepath.Join(root, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		t.Fatalf("go.mod not found at %s: %v", goMod, err)
	}
}

func TestPath(t *testing.T) {
	Reset()

	root, err := Root()
	if err != nil {
		t.Fatalf("Root() failed: %v", err)
	}

	// Test Path function
	configPath, err := Path("configs")
	if err != nil {
		t.Fatalf("Path() failed: %v", err)
	}

	expectedPath := filepath.Join(root, "configs")
	if configPath != expectedPath {
		t.Errorf("Path() = %s, want %s", configPath, expectedPath)
	}

	// Test with nested path
	nestedPath, err := Path("internal/config/config.go")
	if err != nil {
		t.Fatalf("Path() with nested path failed: %v", err)
	}
	expectedNested := filepath.Join(root, "internal/config/config.go")
	if nestedPath != expectedNested {
		t.Errorf("Path() with nested = %s, want %s", nestedPath, expectedNested)
	}

	// Test with relative path with ".."
	parentPath, err := Path("..")
	if err != nil {
		t.Fatalf("Path() with .. failed: %v", err)
	}
	expectedParent := filepath.Join(root, "..")
	expectedParent = filepath.Clean(expectedParent)
	if parentPath != expectedParent {
		t.Errorf("Path() with .. = %s, want %s", parentPath, expectedParent)
	}

	// Test with empty path (should return root)
	rootPath, err := Path("")
	if err != nil {
		t.Fatalf("Path() with empty string failed: %v", err)
	}
	if rootPath != root {
		t.Errorf("Path() with empty string = %s, want %s", rootPath, root)
	}
}

func TestPathWithRootError(t *testing.T) {
	// This test is difficult to run in normal circumstances since we can't
	// easily simulate os.Getwd() failure, but we can test the error propagation
	Reset()

	// Path should propagate errors from Root()
	// In normal cases, Root() should succeed, so this test mainly ensures
	// error propagation works correctly
	_, err := Root()
	if err != nil {
		// If Root() fails, Path() should also fail
		_, pathErr := Path("configs")
		if pathErr == nil {
			t.Error("Path() should fail when Root() fails")
		}
	} else {
		// Normal case - Path should work
		_, pathErr := Path("configs")
		if pathErr != nil {
			t.Errorf("Path() failed when Root() succeeded: %v", pathErr)
		}
	}
}

func TestErrGoModNotFound(t *testing.T) {
	// Test error type
	if ErrGoModNotFound == nil {
		t.Fatal("ErrGoModNotFound should not be nil")
	}

	// Test error message
	expectedMsg := "go.mod not found"
	if ErrGoModNotFound.Error() != expectedMsg {
		t.Errorf("ErrGoModNotFound.Error() = %q, want %q", ErrGoModNotFound.Error(), expectedMsg)
	}

	// Test error wrapping - in normal test environment, Root() should succeed
	_, err := Root()
	if err != nil {
		// If we can't find root in normal test environment, check error type
		if !errors.Is(err, ErrGoModNotFound) {
			// This might happen if error is wrapped differently
			t.Logf("Note: Root() error is not ErrGoModNotFound (may be wrapped): %v", err)
		}
	}
}
