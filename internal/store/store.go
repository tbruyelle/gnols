package store

import (
	"fmt"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	cmap "github.com/orcaman/concurrent-map/v2"
)

// DocumentStore holds all opened documents.
type DocumentStore struct {
	documents cmap.ConcurrentMap[string, *Document]
}

func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		documents: cmap.New[*Document](),
	}
}

func (s *DocumentStore) Save(uri uri.URI, content string) (*Document, error) {
	path, err := s.normalizePath(uri)
	if err != nil {
		return nil, fmt.Errorf("normalize path %s: %w", uri, err)
	}
	pgf, err := NewParsedGnoFile(path, content)
	if err != nil {
		return nil, fmt.Errorf("parse file %s error: %w", path, err)
	}

	doc := &Document{
		URI:     uri,
		Path:    path,
		Content: content,
		Lines:   strings.SplitAfter(content, "\n"),
		Pgf:     pgf,
	}
	s.documents.Set(path, doc)
	return doc, nil
}

func (s *DocumentStore) Close(uri protocol.DocumentURI) {
	s.documents.Remove(uri.Filename())
}

func (s *DocumentStore) Get(docuri uri.URI) (*Document, bool) {
	path, err := s.normalizePath(docuri)
	if err != nil {
		return nil, false
	}
	d, ok := s.documents.Get(path)
	return d, ok
}

func (s *DocumentStore) normalizePath(docuri uri.URI) (string, error) {
	path, err := uriToPath(docuri)
	if err != nil {
		return "", err
	}
	return canonical(path)
}
