package brain

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/coetzeevs/cerebro/internal/store"
)

// Export produces a complete ExportBundle of the brain's contents.
func (b *Brain) Export() (*store.ExportBundle, error) {
	return b.store.Export()
}

// Import loads an ExportBundle into the brain.
// Embeddings are not imported — nodes will need re-embedding.
func (b *Brain) Import(bundle *store.ExportBundle, opts store.ImportOptions) (*store.ImportResult, error) {
	result, err := b.store.Import(bundle, opts)
	if err != nil {
		return nil, err
	}

	if result.NodesImported > 0 {
		_ = b.store.SetMeta("has_pending_embeddings", "true")
	}

	return result, nil
}

// ExportJSON writes the brain as formatted JSON to the writer.
func (b *Brain) ExportJSON(w io.Writer) error {
	return b.store.ExportJSON(w)
}

// ExportSQL writes the brain as SQL INSERT statements to the writer.
func (b *Brain) ExportSQL(w io.Writer) error {
	return b.store.ExportSQL(w)
}

// ExportSQLite copies the database file to the given path.
func (b *Brain) ExportSQLite(dstPath string) error {
	return b.store.ExportSQLite(dstPath)
}

// ImportFromJSON reads a JSON ExportBundle from the reader and imports it.
func (b *Brain) ImportFromJSON(r io.Reader, opts store.ImportOptions) (*store.ImportResult, error) {
	var bundle store.ExportBundle
	dec := json.NewDecoder(r)
	if err := dec.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decoding export bundle: %w", err)
	}
	return b.Import(&bundle, opts)
}
