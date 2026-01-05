package file_utils

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListFiles_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "testdir")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()
	files := []string{"file1.txt", "file2.log", "file3.json"}
	for _, file := range files {
		filePath := fmt.Sprintf("%s/%s", tempDir, file)
		_, err := os.Create(filePath)
		assert.NoError(t, err)
	}
	result, err := ListFiles(tempDir)
	assert.NoError(t, err)
	assert.ElementsMatch(t, files, result)
}

func TestListFiles_DirectoryNotFound(t *testing.T) {
	invalidDir := "/nonexistent/path"
	result, err := ListFiles(invalidDir)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "error reading configuration directory")
}
