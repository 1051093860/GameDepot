package locks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"sort"
	"strings"
	"time"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/store"
	"github.com/1051093860/gamedepot/internal/workspace"
)

const schemaVersion = 1

type Entry struct {
	Version   int    `json:"version"`
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
	Owner     string `json:"owner"`
	Host      string `json:"host"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

type Identity struct {
	Owner string
	Host  string
}

type Manager struct {
	ProjectID string
	Store     store.ObjectStore
}

func NewManager(projectID string, s store.ObjectStore) Manager {
	return Manager{ProjectID: projectID, Store: s}
}

func DefaultIdentity() Identity {
	name := ""
	if cfg, err := config.LoadGlobalConfig(); err == nil {
		name = strings.TrimSpace(cfg.User.Name)
	}
	if name == "" {
		if u, err := user.Current(); err == nil && u.Username != "" {
			name = u.Username
		}
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		name = "unknown"
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown-host"
	}
	return Identity{Owner: name, Host: host}
}

func (m Manager) Lock(ctx context.Context, rel string, identity Identity, note string, force bool) (Entry, bool, error) {
	rel, err := workspace.CleanRelPath(rel)
	if err != nil {
		return Entry{}, false, err
	}
	if identity.Owner == "" {
		identity = DefaultIdentity()
	}

	key := KeyForPath(rel)
	existing, ok, err := m.Get(ctx, rel)
	if err != nil {
		return Entry{}, false, err
	}
	if ok {
		if SameOwner(existing, identity) {
			return existing, false, nil
		}
		if !force {
			return Entry{}, false, fmt.Errorf("already locked: %s by %s@%s at %s; use --force to replace", rel, existing.Owner, existing.Host, existing.CreatedAt)
		}
	}

	entry := Entry{
		Version:   schemaVersion,
		ProjectID: m.ProjectID,
		Path:      rel,
		Owner:     identity.Owner,
		Host:      identity.Host,
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return Entry{}, false, err
	}
	if err := m.Store.PutObject(ctx, key, strings.NewReader(string(data))); err != nil {
		return Entry{}, false, err
	}
	return entry, ok, nil
}

func (m Manager) Unlock(ctx context.Context, rel string, identity Identity, force bool) (Entry, error) {
	rel, err := workspace.CleanRelPath(rel)
	if err != nil {
		return Entry{}, err
	}
	if identity.Owner == "" {
		identity = DefaultIdentity()
	}

	entry, ok, err := m.Get(ctx, rel)
	if err != nil {
		return Entry{}, err
	}
	if !ok {
		return Entry{}, fmt.Errorf("not locked: %s", rel)
	}
	if !SameOwner(entry, identity) && !force {
		return Entry{}, fmt.Errorf("lock is owned by %s@%s; use --force to unlock", entry.Owner, entry.Host)
	}
	if err := m.Store.DeleteObject(ctx, KeyForPath(rel)); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (m Manager) Get(ctx context.Context, rel string) (Entry, bool, error) {
	rel, err := workspace.CleanRelPath(rel)
	if err != nil {
		return Entry{}, false, err
	}
	key := KeyForPath(rel)
	ok, err := m.Store.HasObject(ctx, key)
	if err != nil {
		return Entry{}, false, err
	}
	if !ok {
		return Entry{}, false, nil
	}
	r, err := m.Store.GetObject(ctx, key)
	if err != nil {
		return Entry{}, false, err
	}
	defer r.Close()
	entry, err := decodeEntry(r)
	if err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

func (m Manager) List(ctx context.Context) ([]Entry, error) {
	keys, err := m.Store.ListObjects(ctx, "locks/")
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		if !strings.HasSuffix(key, ".json") {
			continue
		}
		r, err := m.Store.GetObject(ctx, key)
		if err != nil {
			return nil, err
		}
		entry, decErr := decodeEntry(r)
		closeErr := r.Close()
		if decErr != nil {
			return nil, decErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if m.ProjectID == "" || entry.ProjectID == "" || entry.ProjectID == m.ProjectID {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func KeyForPath(rel string) string {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.Trim(rel, "/")
	sum := sha256.Sum256([]byte(rel))
	h := hex.EncodeToString(sum[:])
	return "locks/" + h[:2] + "/" + h + ".json"
}

func SameOwner(entry Entry, identity Identity) bool {
	return strings.EqualFold(strings.TrimSpace(entry.Owner), strings.TrimSpace(identity.Owner)) && strings.EqualFold(strings.TrimSpace(entry.Host), strings.TrimSpace(identity.Host))
}

func decodeEntry(r io.Reader) (Entry, error) {
	var entry Entry
	data, err := io.ReadAll(r)
	if err != nil {
		return Entry{}, err
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		return Entry{}, err
	}
	if entry.Version == 0 {
		entry.Version = schemaVersion
	}
	return entry, nil
}
