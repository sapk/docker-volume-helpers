package tools

import (
	"io"
	"os"
)

//FolderIsEmpty based on: http://stackoverflow.com/questions/30697324/how-to-check-if-directory-on-path-is-empty
func FolderIsEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}
