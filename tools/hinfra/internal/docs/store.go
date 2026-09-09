package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	Root string
}

func NewStore(infraRepo string) *Store {
	return &Store{Root: filepath.Join(infraRepo, "docs")}
}

func (s *Store) Read(rel string) (string, error) {
	rel = filepath.Clean(rel)
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("caminho inválido: %s", rel)
	}
	path := filepath.Join(s.Root, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type SearchResult struct {
	File    string
	Line    int
	Snippet string
}

func (s *Store) Search(query string) ([]SearchResult, error) {
	var results []SearchResult
	query = strings.ToLower(query)
	err := filepath.Walk(s.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".html") && !strings.HasSuffix(path, ".yml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(s.Root, path)
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), query) {
				results = append(results, SearchResult{
					File:    rel,
					Line:    i + 1,
					Snippet: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})
	return results, err
}
