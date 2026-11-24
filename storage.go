package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

const (
	profilesFile    = "profiles.json"
	macrosFile      = "macros.json"
	preferencesFile = "preferences.json"
)

// SavedProfile is a local snapshot of one onboard configuration.
type SavedProfile struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	UpdatedAt time.Time       `json:"updated_at"`
	DPI       hid.DPIConfig   `json:"dpi"`
	Params    hid.MouseParams `json:"params"`
	Keys      [16][4]byte     `json:"keys"`
}

// SavedMacro is a named macro kept in the local library (and optionally a mouse slot).
type SavedMacro struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slot      uint8     `json:"slot"`
	Macro     hid.Macro `json:"macro"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AppPreferences holds UI preferences that survive restarts.
type AppPreferences struct {
	ThemeMode string `json:"theme_mode"` // "system", "dark", "light"
}

// LocalStore persists profiles, macros, and preferences under dir.
// The default directory is ~/.config/valkyrie-driver/.
type LocalStore struct {
	mu  sync.Mutex
	dir string
}

// DefaultConfigDir returns ~/.config/valkyrie-driver.
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "valkyrie-driver"), nil
}

// NewLocalStore opens a store rooted at dir. An empty dir uses DefaultConfigDir.
func NewLocalStore(dir string) (*LocalStore, error) {
	if dir == "" {
		var err error
		dir, err = DefaultConfigDir()
		if err != nil {
			return nil, err
		}
	}
	return &LocalStore{dir: dir}, nil
}

func (s *LocalStore) ensure() error {
	return os.MkdirAll(s.dir, 0o700)
}

func (s *LocalStore) loadJSON(name string, dest any) error {
	path := filepath.Join(s.dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dest)
}

func (s *LocalStore) saveJSON(name string, v any) error {
	if err := s.ensure(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newStoreID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// LoadProfiles returns every saved profile. A missing file yields an empty list.
func (s *LocalStore) LoadProfiles() ([]SavedProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []SavedProfile
	if err := s.loadJSON(profilesFile, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []SavedProfile{}
	}
	return items, nil
}

// SaveProfile inserts or replaces a profile by ID. An empty ID is assigned.
func (s *LocalStore) SaveProfile(p SavedProfile) (SavedProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []SavedProfile
	if err := s.loadJSON(profilesFile, &items); err != nil {
		return SavedProfile{}, err
	}
	if p.ID == "" {
		p.ID = newStoreID()
	}
	if p.Name == "" {
		p.Name = "未命名配置"
	}
	p.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	replaced := false
	for i := range items {
		if items[i].ID == p.ID {
			items[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, p)
	}
	if err := s.saveJSON(profilesFile, items); err != nil {
		return SavedProfile{}, err
	}
	return p, nil
}

// DeleteProfile removes a profile by ID.
func (s *LocalStore) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []SavedProfile
	if err := s.loadJSON(profilesFile, &items); err != nil {
		return err
	}
	next := items[:0]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return fmt.Errorf("配置不存在: %s", id)
	}
	if next == nil {
		next = []SavedProfile{}
	}
	return s.saveJSON(profilesFile, next)
}

// RenameProfile changes the display name of a saved profile.
func (s *LocalStore) RenameProfile(id, name string) (SavedProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		return SavedProfile{}, errors.New("配置名称不能为空")
	}
	var items []SavedProfile
	if err := s.loadJSON(profilesFile, &items); err != nil {
		return SavedProfile{}, err
	}
	for i := range items {
		if items[i].ID == id {
			items[i].Name = name
			items[i].UpdatedAt = time.Now().UTC().Truncate(time.Second)
			if err := s.saveJSON(profilesFile, items); err != nil {
				return SavedProfile{}, err
			}
			return items[i], nil
		}
	}
	return SavedProfile{}, fmt.Errorf("配置不存在: %s", id)
}

// ExportProfileJSON returns the pretty JSON of one saved profile.
func (s *LocalStore) ExportProfileJSON(id string) ([]byte, error) {
	items, err := s.LoadProfiles()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			data, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				return nil, err
			}
			return append(data, '\n'), nil
		}
	}
	return nil, fmt.Errorf("配置不存在: %s", id)
}

// ImportProfileJSON parses a profile and stores it. A colliding ID is kept and overwritten.
func (s *LocalStore) ImportProfileJSON(data []byte) (SavedProfile, error) {
	var p SavedProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return SavedProfile{}, fmt.Errorf("配置 JSON 无效: %w", err)
	}
	return s.SaveProfile(p)
}

// LoadMacros returns the local macro library.
func (s *LocalStore) LoadMacros() ([]SavedMacro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []SavedMacro
	if err := s.loadJSON(macrosFile, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []SavedMacro{}
	}
	return items, nil
}

// SaveMacro inserts or replaces a macro by ID.
func (s *LocalStore) SaveMacro(m SavedMacro) (SavedMacro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.Slot > 15 {
		return SavedMacro{}, fmt.Errorf("宏槽位超出范围: %d", m.Slot)
	}
	var items []SavedMacro
	if err := s.loadJSON(macrosFile, &items); err != nil {
		return SavedMacro{}, err
	}
	if m.ID == "" {
		m.ID = newStoreID()
	}
	if m.Name == "" {
		m.Name = "未命名宏"
	}
	m.Macro.Name = m.Name
	m.Macro.Slot = m.Slot
	m.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	replaced := false
	for i := range items {
		if items[i].ID == m.ID {
			items[i] = m
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, m)
	}
	if err := s.saveJSON(macrosFile, items); err != nil {
		return SavedMacro{}, err
	}
	return m, nil
}

// DeleteMacro removes a library macro by ID.
func (s *LocalStore) DeleteMacro(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []SavedMacro
	if err := s.loadJSON(macrosFile, &items); err != nil {
		return err
	}
	next := items[:0]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return fmt.Errorf("宏不存在: %s", id)
	}
	if next == nil {
		next = []SavedMacro{}
	}
	return s.saveJSON(macrosFile, next)
}

// LoadPreferences returns saved UI preferences, or the system theme when unset.
func (s *LocalStore) LoadPreferences() (AppPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefs := AppPreferences{ThemeMode: "system"}
	if err := s.loadJSON(preferencesFile, &prefs); err != nil {
		return AppPreferences{}, err
	}
	if prefs.ThemeMode == "" {
		prefs.ThemeMode = "system"
	}
	return prefs, nil
}

// SavePreferences writes the theme preference.
func (s *LocalStore) SavePreferences(prefs AppPreferences) error {
	switch prefs.ThemeMode {
	case "system", "dark", "light":
	default:
		return fmt.Errorf("未知主题: %q", prefs.ThemeMode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveJSON(preferencesFile, prefs)
}
