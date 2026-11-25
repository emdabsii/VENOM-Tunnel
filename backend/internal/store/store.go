package store

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/venom-tunnel/venom/internal/config"
)

type TunnelRecord struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Engine       string            `json:"engine"`
	Role         string            `json:"role"`
	LocalIP      string            `json:"local_ip"`
	RemoteIP     string            `json:"remote_ip"`
	ControlPort  int               `json:"control_port"`
	Ports        string            `json:"ports"`
	Token        string            `json:"token"`
	Extra        map[string]string `json:"extra"`
	ServiceName  string            `json:"service_name"`
	ConfigPath   string            `json:"config_path"`
	Imported     bool              `json:"imported"`
	AutoWatchdog bool              `json:"auto_watchdog"`
	State        string            `json:"state"`
	RTTMs        float64           `json:"rtt_ms"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

type AuditEntry struct {
	Time   string `json:"time"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

type StateData struct {
	Tunnels []TunnelRecord `json:"tunnels"`
	Audits  []AuditEntry   `json:"audits"`
}

type Store struct {
	mu    sync.RWMutex
	state StateData
}

func New() *Store {
	s := &Store{}
	s.initSQLite()
	s.load()
	return s
}

func (s *Store) initSQLite() {
	schemaPath := "/opt/venom/sql/schema.sql"
	if _, err := os.Stat(schemaPath); err != nil {
		return
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return
	}
	cmd := exec.Command("sqlite3", config.DBFile)
	if f, err := os.Open(schemaPath); err == nil {
		cmd.Stdin = f
		_ = cmd.Run()
		_ = f.Close()
	}
}

func (s *Store) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(config.StateFile)
	if err != nil {
		s.state = StateData{Tunnels: []TunnelRecord{}, Audits: []AuditEntry{}}
		return
	}
	_ = json.Unmarshal(b, &s.state)
	if s.state.Tunnels == nil {
		s.state.Tunnels = []TunnelRecord{}
	}
	if s.state.Audits == nil {
		s.state.Audits = []AuditEntry{}
	}
}

func (s *Store) saveLocked() error {
	_ = os.MkdirAll(filepath.Dir(config.StateFile), 0700)
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(config.StateFile, b, 0600)
}

func (s *Store) ListTunnels() []TunnelRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TunnelRecord, len(s.state.Tunnels))
	copy(out, s.state.Tunnels)
	return out
}

func (s *Store) GetTunnel(name string) (*TunnelRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.state.Tunnels {
		if t.Name == name || t.ID == name {
			cp := t
			return &cp, true
		}
	}
	return nil, false
}

func (s *Store) UpsertTunnel(t TunnelRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	t.UpdatedAt = now
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	for i := range s.state.Tunnels {
		if s.state.Tunnels[i].Name == t.Name {
			s.state.Tunnels[i] = t
			return s.saveLocked()
		}
	}
	s.state.Tunnels = append(s.state.Tunnels, t)
	return s.saveLocked()
}

func (s *Store) DeleteTunnel(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]TunnelRecord, 0, len(s.state.Tunnels))
	for _, t := range s.state.Tunnels {
		if t.Name != name {
			filtered = append(filtered, t)
		}
	}
	s.state.Tunnels = filtered
	return s.saveLocked()
}

func (s *Store) AddAudit(actor, action, target, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Audits = append([]AuditEntry{{
		Time:   time.Now().UTC().Format(time.RFC3339),
		Actor:  actor,
		Action: action,
		Target: target,
		Detail: detail,
	}}, s.state.Audits...)
	if len(s.state.Audits) > 200 {
		s.state.Audits = s.state.Audits[:200]
	}
	_ = s.saveLocked()
}

func (s *Store) ListAudits() []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditEntry, len(s.state.Audits))
	copy(out, s.state.Audits)
	return out
}
