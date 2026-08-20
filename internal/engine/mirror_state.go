package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// state records the last sync's package set for incremental updates.
type state struct {
	Revision string            `json:"revision"`
	SyncedAt time.Time         `json:"synced_at"`
	Packages map[string]string `json:"packages"` // location -> checksum
}

// matches reports whether the recorded checksum for loc equals given unless empty.
func (s state) matches(loc, checksum string) bool {
	return s.Packages[loc] == checksum
}

func loadState(root string) state {
	path := filepath.Join(root, "mirror-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return state{Packages: map[string]string{}}
	}
	var s state
	if json.Unmarshal(data, &s) != nil || s.Packages == nil {
		return state{Packages: map[string]string{}}
	}
	return s
}

func saveState(root string, s state) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "mirror-state.json"), data, 0o644)
}
