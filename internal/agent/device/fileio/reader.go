package fileio

import (
	"fmt"
	"io/fs"
	"os"
	"path"
)

// Reader is a struct for reading files from the device
type reader struct {
	// rootDir is the root directory for the device writer useful for testing
	rootDir string
}

// New creates a new writer
func NewReader() *reader {
	return &reader{}
}

// SetRootdir sets the root directory for the reader, useful for testing
func (r *reader) SetRootdir(path string) {
	r.rootDir = path
}

// PathFor returns the full path for the provided file, useful for using functions
// and libraries that don't work with the fileio.Reader
func (r *reader) PathFor(filePath string) string {
	return path.Join(r.rootDir, filePath)
}

// ReadFile reads the file at the provided path
func (r *reader) ReadFile(filePath string) ([]byte, error) {
	return os.ReadFile(r.PathFor(filePath))
}

// ReadDir reads the directory at the provided path and returns a slice of fs.DirEntry. If the directory
// does not exist, it returns an empty slice and no error.
func (r *reader) ReadDir(dirPath string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(r.PathFor(dirPath))
	if err != nil {
		if os.IsNotExist(err) {
			return []fs.DirEntry{}, nil
		}
		return nil, err
	}
	return entries, nil
}

// FileExists checks if a path exists and returns a boolean indicating existence,
// and an error only if there was a problem checking the path.
func (r *reader) FileExists(filePath string) (bool, error) {
	return checkPathExists(r.PathFor(filePath))
}

func checkPathExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("error checking path: %w", err)
}
