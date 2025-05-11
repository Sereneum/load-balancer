package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {

	p, _ := os.Getwd()
	fmt.Println(p)

	if err := DumpGoProject(".", "project_code.txt"); err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Code dump saved to project_code.txt")
	}
}

// DumpGoProject собирает все .go файлы в модуле и сохраняет их в один текстовый файл.
func DumpGoProject(rootDir string, outputFile string) error {
	moduleRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot find go.mod: %w", err)
	}

	var buffer bytes.Buffer

	err = filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			relPath, _ := filepath.Rel(moduleRoot, path)
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("error reading %s: %w", path, err)
			}

			buffer.WriteString(fmt.Sprintf("==== ./%s ====\n", strings.Replace(relPath, "\\", "/", -1)))
			buffer.Write(content)
			buffer.WriteString("\n\n")
		}
		return nil
	})
	if err != nil {
		return err
	}

	return os.WriteFile(outputFile, buffer.Bytes(), 0644)
}

// findGoModRoot ищет корень модуля Go (директорию с go.mod).
func findGoModRoot(startDir string) (string, error) {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}
