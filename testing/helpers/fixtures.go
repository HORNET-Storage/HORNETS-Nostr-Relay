// Package helpers provides utilities for integration testing the HORNETS relay
package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
)

// TestFixture represents a deterministic test data structure
type TestFixture struct {
	Name           string
	Description    string
	Setup          func(baseDir string) error
	ExpectedFiles  int
	ExpectedDirs   int
	ExpectedChunks int
}

// GetAllFixtures returns all available test fixtures
func GetAllFixtures() []TestFixture {
	return []TestFixture{
		SingleSmallFile(),
		SingleLargeFile(),
		FlatDirectory(),
		NestedDirectory(),
		DeepHierarchy(),
		MixedSizes(),
		ManySmallFiles(),
	}
}

// SingleSmallFile creates a single file well below the chunk size (4KB default)
// Use case: Testing basic file DAG creation, single leaf DAGs
func SingleSmallFile() TestFixture {
	return TestFixture{
		Name:        "single_small_file",
		Description: "Single 1KB file - no chunking",
		Setup: func(baseDir string) error {
			filePath := filepath.Join(baseDir, "small.txt")
			content := makeDeterministicContent(1024, 0)
			return os.WriteFile(filePath, content, 0644)
		},
		ExpectedFiles:  1,
		ExpectedDirs:   0,
		ExpectedChunks: 0,
	}
}

// SingleLargeFile creates a single file above the chunk size requiring chunking
// Use case: Testing file chunking, merkle tree construction for chunks
func SingleLargeFile() TestFixture {
	return TestFixture{
		Name:        "single_large_file",
		Description: "Single 10KB file - requires chunking (default chunk size 4KB)",
		Setup: func(baseDir string) error {
			filePath := filepath.Join(baseDir, "large.txt")
			content := makeDeterministicContent(10*1024, 0)
			return os.WriteFile(filePath, content, 0644)
		},
		ExpectedFiles:  1,
		ExpectedDirs:   0,
		ExpectedChunks: 3, // 10KB / 4KB = 3 chunks
	}
}

// FlatDirectory creates a directory with multiple files at the same level
// Use case: Testing parent-child relationships, merkle proofs for siblings
func FlatDirectory() TestFixture {
	return TestFixture{
		Name:        "flat_directory",
		Description: "One directory with 5 small files (no subdirectories)",
		Setup: func(baseDir string) error {
			files := []struct {
				name string
				size int
			}{
				{"file1.txt", 512},
				{"file2.txt", 1024},
				{"file3.txt", 768},
				{"file4.txt", 2048},
				{"file5.txt", 256},
			}

			for i, f := range files {
				filePath := filepath.Join(baseDir, f.name)
				content := makeDeterministicContent(f.size, i)
				if err := os.WriteFile(filePath, content, 0644); err != nil {
					return err
				}
			}
			return nil
		},
		ExpectedFiles:  5,
		ExpectedDirs:   0,
		ExpectedChunks: 0,
	}
}

// NestedDirectory creates a two-level directory structure
// Use case: Testing directory traversal, multiple parent-child levels
func NestedDirectory() TestFixture {
	return TestFixture{
		Name:        "nested_directory",
		Description: "Two-level hierarchy: root -> 2 subdirs -> 2 files each",
		Setup: func(baseDir string) error {
			structure := []struct {
				dir   string
				files []string
			}{
				{"subdir1", []string{"file1a.txt", "file1b.txt"}},
				{"subdir2", []string{"file2a.txt", "file2b.txt"}},
			}

			seed := 0
			for _, s := range structure {
				dirPath := filepath.Join(baseDir, s.dir)
				if err := os.MkdirAll(dirPath, 0755); err != nil {
					return err
				}

				for i, fileName := range s.files {
					filePath := filepath.Join(dirPath, fileName)
					content := makeDeterministicContent(1024+i*512, seed)
					seed++
					if err := os.WriteFile(filePath, content, 0644); err != nil {
						return err
					}
				}
			}
			return nil
		},
		ExpectedFiles:  4,
		ExpectedDirs:   2,
		ExpectedChunks: 0,
	}
}

// DeepHierarchy creates a deeply nested directory structure
// Use case: Testing deep path traversal, verification paths through multiple levels
func DeepHierarchy() TestFixture {
	return TestFixture{
		Name:        "deep_hierarchy",
		Description: "Five-level deep directory structure with files at each level",
		Setup: func(baseDir string) error {
			// Create a 5-level deep structure: level1/level2/level3/level4/level5/file.txt
			deepPath := filepath.Join(baseDir, "level1", "level2", "level3", "level4", "level5")
			if err := os.MkdirAll(deepPath, 0755); err != nil {
				return err
			}

			// Add a file at each level
			for i := 1; i <= 5; i++ {
				levelPath := filepath.Join(baseDir, "level1")
				for j := 2; j <= i; j++ {
					levelPath = filepath.Join(levelPath, fmt.Sprintf("level%d", j))
				}

				filePath := filepath.Join(levelPath, fmt.Sprintf("file_at_level_%d.txt", i))
				content := makeDeterministicContent(i*256, i)
				if err := os.WriteFile(filePath, content, 0644); err != nil {
					return err
				}
			}

			return nil
		},
		ExpectedFiles:  5,
		ExpectedDirs:   5,
		ExpectedChunks: 0,
	}
}

// MixedSizes creates a structure with both small and large files requiring chunking
// Use case: Testing mixed scenarios with and without chunking
func MixedSizes() TestFixture {
	return TestFixture{
		Name:        "mixed_sizes",
		Description: "Directory with both small files and large files requiring chunking",
		Setup: func(baseDir string) error {
			files := []struct {
				name string
				size int
			}{
				{"tiny.txt", 128},             // Very small
				{"small.txt", 2048},           // Below chunk size
				{"medium.txt", 5 * 1024},      // Requires 2 chunks
				{"large.txt", 15 * 1024},      // Requires 4 chunks
				{"exact_chunk.txt", 4 * 1024}, // Exactly one chunk
			}

			for i, f := range files {
				filePath := filepath.Join(baseDir, f.name)
				content := makeDeterministicContent(f.size, i)
				if err := os.WriteFile(filePath, content, 0644); err != nil {
					return err
				}
			}
			return nil
		},
		ExpectedFiles:  5,
		ExpectedDirs:   0,
		ExpectedChunks: 7, // 0 + 0 + 2 + 4 + 1
	}
}

// ManySmallFiles creates a directory with many small files
// Use case: Testing batching, many sibling merkle proofs
func ManySmallFiles() TestFixture {
	return TestFixture{
		Name:        "many_small_files",
		Description: "Directory with 20 small files to test batching",
		Setup: func(baseDir string) error {
			for i := 0; i < 20; i++ {
				filePath := filepath.Join(baseDir, fmt.Sprintf("file_%02d.txt", i))
				content := makeDeterministicContent(512, i)
				if err := os.WriteFile(filePath, content, 0644); err != nil {
					return err
				}
			}
			return nil
		},
		ExpectedFiles:  20,
		ExpectedDirs:   0,
		ExpectedChunks: 0,
	}
}

// makeDeterministicContent creates deterministic content of a given size
// The seed ensures different files have different content
func makeDeterministicContent(size int, seed int) []byte {
	content := make([]byte, size)
	for i := range content {
		// Deterministic pattern based on position and seed
		content[i] = byte('A' + ((i + seed) % 26))
	}
	return content
}

// CreateFixture creates a test fixture in the specified directory
// Returns the path to the created fixture directory
func CreateFixture(baseDir string, fixture TestFixture) (string, error) {
	fixturePath := filepath.Join(baseDir, fixture.Name)
	if err := os.MkdirAll(fixturePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create fixture directory: %w", err)
	}

	if err := fixture.Setup(fixturePath); err != nil {
		os.RemoveAll(fixturePath)
		return "", fmt.Errorf("failed to setup fixture %s: %w", fixture.Name, err)
	}

	return fixturePath, nil
}

// GetFixtureByName returns a specific fixture by name
func GetFixtureByName(name string) (TestFixture, bool) {
	for _, f := range GetAllFixtures() {
		if f.Name == name {
			return f, true
		}
	}
	return TestFixture{}, false
}

// GetMultiFileFixtures returns fixtures that have multiple files (useful for partial DAG tests)
func GetMultiFileFixtures() []TestFixture {
	return []TestFixture{
		FlatDirectory(),
		NestedDirectory(),
		DeepHierarchy(),
		MixedSizes(),
		ManySmallFiles(),
	}
}

// GetSingleFileFixtures returns fixtures with only one file
func GetSingleFileFixtures() []TestFixture {
	return []TestFixture{
		SingleSmallFile(),
		SingleLargeFile(),
	}
}

// GetChunkingFixtures returns fixtures that test chunking behavior
func GetChunkingFixtures() []TestFixture {
	return []TestFixture{
		SingleLargeFile(),
		MixedSizes(),
	}
}

// GetHierarchyFixtures returns fixtures with nested directory structures
func GetHierarchyFixtures() []TestFixture {
	return []TestFixture{
		NestedDirectory(),
		DeepHierarchy(),
	}
}

// FixtureDAG holds a DAG created from a fixture along with metadata
type FixtureDAG struct {
	Dag         *merkle_dag.Dag
	Fixture     TestFixture
	FixturePath string
	TempDir     string
}

// Cleanup removes the temporary directory
func (fd *FixtureDAG) Cleanup() error {
	if fd.TempDir != "" {
		return os.RemoveAll(fd.TempDir)
	}
	return nil
}

// CreateDAGFromFixture creates a DAG from a specific fixture
func CreateDAGFromFixture(fixture TestFixture) (*FixtureDAG, error) {
	tmpDir, err := os.MkdirTemp("", "dag-fixture-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	fixturePath, err := CreateFixture(tmpDir, fixture)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	// Use parallel config for DAG creation
	config := merkle_dag.ParallelConfig()
	dag, err := merkle_dag.CreateDagWithConfig(fixturePath, config)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to create DAG from fixture: %w", err)
	}

	return &FixtureDAG{
		Dag:         dag,
		Fixture:     fixture,
		FixturePath: fixturePath,
		TempDir:     tmpDir,
	}, nil
}

// CreateDAGFromFixtureName creates a DAG from a fixture by name
func CreateDAGFromFixtureName(name string) (*FixtureDAG, error) {
	fixture, ok := GetFixtureByName(name)
	if !ok {
		return nil, fmt.Errorf("fixture not found: %s", name)
	}
	return CreateDAGFromFixture(fixture)
}

// RunTestWithFixture runs a test function against a specific fixture
func RunTestWithFixture(t *testing.T, fixtureName string, testFunc func(*testing.T, *FixtureDAG)) {
	fixture, ok := GetFixtureByName(fixtureName)
	if !ok {
		t.Fatalf("Fixture not found: %s", fixtureName)
	}

	fd, err := CreateDAGFromFixture(fixture)
	if err != nil {
		t.Fatalf("Failed to create DAG from fixture: %v", err)
	}
	defer fd.Cleanup()

	testFunc(t, fd)
}

// RunTestWithAllFixtures runs a test function against all fixtures
func RunTestWithAllFixtures(t *testing.T, testFunc func(*testing.T, *FixtureDAG)) {
	for _, fixture := range GetAllFixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			fd, err := CreateDAGFromFixture(fixture)
			if err != nil {
				t.Fatalf("Failed to create DAG from fixture: %v", err)
			}
			defer fd.Cleanup()

			testFunc(t, fd)
		})
	}
}

// RunTestWithMultiFileFixtures runs a test against fixtures with multiple files
func RunTestWithMultiFileFixtures(t *testing.T, testFunc func(*testing.T, *FixtureDAG)) {
	for _, fixture := range GetMultiFileFixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			fd, err := CreateDAGFromFixture(fixture)
			if err != nil {
				t.Fatalf("Failed to create DAG from fixture: %v", err)
			}
			defer fd.Cleanup()

			testFunc(t, fd)
		})
	}
}

// GetFileLeafHashes returns all file leaf hashes from a DAG in a deterministic order
func GetFileLeafHashes(dag *merkle_dag.Dag) []string {
	var hashes []string
	for hash, leaf := range dag.Leafs {
		if leaf.Type == merkle_dag.FileLeafType {
			hashes = append(hashes, hash)
		}
	}
	// Sort for deterministic ordering
	sortStrings(hashes)
	return hashes
}

// GetDirectoryLeafHashes returns all directory leaf hashes from a DAG in a deterministic order
func GetDirectoryLeafHashes(dag *merkle_dag.Dag) []string {
	var hashes []string
	for hash, leaf := range dag.Leafs {
		if leaf.Type == merkle_dag.DirectoryLeafType {
			hashes = append(hashes, hash)
		}
	}
	// Sort for deterministic ordering
	sortStrings(hashes)
	return hashes
}

// sortStrings sorts a slice of strings in place (simple insertion sort for small slices)
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// GetLeafTypeCount returns a map of leaf types to their counts (for debugging)
func GetLeafTypeCount(dag *merkle_dag.Dag) map[string]int {
	counts := make(map[string]int)
	for _, leaf := range dag.Leafs {
		counts[string(leaf.Type)]++
	}
	return counts
}
