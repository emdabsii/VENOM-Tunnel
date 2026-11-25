package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/venom-tunnel/venom/internal/config"
	"github.com/venom-tunnel/venom/internal/store"
	"github.com/venom-tunnel/venom/internal/tunnel"
)

type Server struct {
	Cfg      *config.PanelConfig
	Store    *store.Store
	WebFS    embed.FS
	ServerIP string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	prefix := "/" + strings.Trim(s.Cfg.SecretPath, "/")

	// Auth endpoints
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/me", s.requireAuth(s.handleMe))

	// Protected API endpoints
	mux.HandleFunc("/api/system/telemetry", s.requireAuth(s.handleTelemetry))
	mux.HandleFunc("/api/system/intl", s.requireAuth(s.handleIntlCheck))
	mux.HandleFunc("/api/system/probe", s.requireAuth(s.handleTCPProbe))
	mux.HandleFunc("/api/system/optimize", s.requireAuth(s.handleOptimize))
	mux.HandleFunc("/api/tunnels", s.requireAuth(s.handleTunnels))
	mux.HandleFunc("/api/tunnels/pair", s.requireAuth(s.handlePairApply))
	mux.HandleFunc("/api/tunnels/action", s.requireAuth(s.handleTunnelAction))
	mux.HandleFunc("/api/tunnels/logs", s.requireAuth(s.handleTunnelLogs))
	mux.HandleFunc("/api/tunnels/export", s.requireAuth(s.handleExport))
	mux.HandleFunc("/api/settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("/api/audits", s.requireAuth(s.handleAudits))

	// Embedded graphical web panel under secret path
	sub, _ := fs.Sub(s.WebFS, "webdist")
	fileSrv := http.FileServer(http.FS(sub))
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prefix+"/", http.StatusMovedPermanently)
	})
	mux.Handle(prefix+"/", http.StripPrefix(prefix+"/", fileSrv))

	// Hide root path with 404 stealth response
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Error(w, "404 page not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	})

	return withSecurityHeaders(mux)
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) signToken(user string) string {
	exp := time.Now().Add(24 * time.Hour).Unix()
	payload := fmt.Sprintf("%s|%d", user, exp)
	mac := hmac.New(sha256.New, []byte(s.Cfg.JWTSecret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func (s *Server) verifyToken(tok string) (string, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(s.Cfg.JWTSecret))
	mac.Write(raw)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", false
	}
	fields := strings.Split(string(raw), "|")
	if len(fields) != 2 {
		return "", false
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return fields[0], true
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if auth == "" {
			if c, err := r.Cookie("venom_session"); err == nil {
				auth = c.Value
			}
		}
		if _, ok := s.verifyToken(auth); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if req.Username != s.Cfg.Username || config.HashPassword(req.Password, s.Cfg.PasswordSalt) != s.Cfg.PasswordHash {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	tok := s.signToken(req.Username)
	http.SetCookie(w, &http.Cookie{
		Name: "venom_session", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400,
	})
	s.Store.AddAudit(req.Username, "login", "panel", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "username": req.Username, "server_ip": s.ServerIP})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"username":      s.Cfg.Username,
		"server_ip":     s.ServerIP,
		"port":          s.Cfg.Port,
		"default_lang":  s.Cfg.DefaultLang,
		"default_theme": s.Cfg.DefaultTheme,
		"version":       "2.0.0",
	})
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("/usr/local/bin/venom-netprobe", "telemetry").Output()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"cpu_pct": 0, "ram_pct": 0, "rx_mbps": 0, "tx_mbps": 0})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *Server) handleIntlCheck(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("/usr/local/bin/venom-netprobe", "intl").Output()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"international_open": false, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *Server) handleTCPProbe(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	port := r.URL.Query().Get("port")
	count := r.URL.Query().Get("count")
	if host == "" || port == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host and port required"})
		return
	}
	if count == "" {
		count = "10"
	}
	out, err := exec.Command("/usr/local/bin/venom-netprobe", "tcp", host, port, count).Output()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *Server) handleOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		out, _ := exec.Command("/usr/local/bin/venom-sysopt", "status").Output()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out)
		return
	}
	out, err := exec.Command("/usr/local/bin/venom-sysopt", "apply", "eth0").Output()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.Store.AddAudit(s.Cfg.Username, "optimize", "kernel", "Applied BBR + FQ + RPS")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *Server) handleTunnels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListTunnels()
		for i := range list {
			list[i].State = tunnel.ServiceState(list[i].ServiceName)
		}
		writeJSON(w, http.StatusOK, map[string]any{"tunnels": list, "server_ip": s.ServerIP})
	case http.MethodPost, http.MethodPut:
		var rec store.TunnelRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		if rec.LocalIP == "" {
			rec.LocalIP = s.ServerIP
		}
		if existing, ok := s.Store.GetTunnel(rec.Name); ok {
			rec.ID = existing.ID
			rec.ServiceName = existing.ServiceName
			rec.ConfigPath = existing.ConfigPath
			rec.Imported = existing.Imported
		}
		if err := tunnel.BuildAndApply(&rec); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		rec.State = tunnel.ServiceState(rec.ServiceName)
		_ = s.Store.UpsertTunnel(rec)
		s.Store.AddAudit(s.Cfg.Username, "upsert-tunnel", rec.Name, rec.Engine+" ("+rec.Role+")")
		writeJSON(w, http.StatusOK, map[string]any{"tunnel": rec, "pairing_code": tunnel.MakePairingCode(rec)})
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		rec, ok := s.Store.GetTunnel(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
			return
		}
		_ = tunnel.Remove(rec)
		_ = s.Store.DeleteTunnel(name)
		s.Store.AddAudit(s.Cfg.Username, "delete-tunnel", name, rec.Engine)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handlePairApply(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		name := r.URL.Query().Get("name")
		rec, ok := s.Store.GetTunnel(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
			return
		}
		code := tunnel.MakePairingCode(*rec)
		writeJSON(w, http.StatusOK, map[string]string{"pairing_code": code, "cli_command": "venom pair '" + code + "'"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	p, err := tunnel.DecodePairingCode(req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing code"})
		return
	}
	rec := store.TunnelRecord{
		Name:         p.Name,
		Engine:       p.Engine,
		Role:         p.Role,
		LocalIP:      s.ServerIP,
		RemoteIP:     p.RemoteIP,
		ControlPort:  p.ControlPort,
		Ports:        p.Ports,
		Token:        p.Token,
		AutoWatchdog: true,
	}
	if err := tunnel.BuildAndApply(&rec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rec.State = tunnel.ServiceState(rec.ServiceName)
	_ = s.Store.UpsertTunnel(rec)
	s.Store.AddAudit(s.Cfg.Username, "pair-tunnel", rec.Name, "Paired with "+rec.RemoteIP)
	writeJSON(w, http.StatusOK, map[string]any{"tunnel": rec})
}

func (s *Server) handleTunnelAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	rec, ok := s.Store.GetTunnel(req.Name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
		return
	}
	if err := tunnel.Action(rec.ServiceName, req.Action); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.Store.AddAudit(s.Cfg.Username, req.Action, req.Name, rec.ServiceName)
	writeJSON(w, http.StatusOK, map[string]string{"state": tunnel.ServiceState(rec.ServiceName)})
}

func (s *Server) handleTunnelLogs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	rec, ok := s.Store.GetTunnel(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": tunnel.Logs(rec.ServiceName, 80)})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"tunnels":     s.Store.ListTunnels(),
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{
			"port":             s.Cfg.Port,
			"secret_path":      s.Cfg.SecretPath,
			"username":         s.Cfg.Username,
			"default_lang":     s.Cfg.DefaultLang,
			"default_theme":    s.Cfg.DefaultTheme,
			"domain":           s.Cfg.Domain,
			"telegram_chat_id": s.Cfg.TelegramChatID,
			"watchdog_sec":     s.Cfg.WatchdogSec,
		})
		return
	}
	var req struct {
		Port           int    `json:"port"`
		SecretPath     string `json:"secret_path"`
		Username       string `json:"username"`
		NewPassword    string `json:"new_password"`
		DefaultLang    string `json:"default_lang"`
		DefaultTheme   string `json:"default_theme"`
		Domain         string `json:"domain"`
		TelegramToken  string `json:"telegram_token"`
		TelegramChatID string `json:"telegram_chat_id"`
		WatchdogSec    int    `json:"watchdog_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if req.Port >= 10000 && req.Port <= 65535 {
		s.Cfg.Port = req.Port
	}
	if req.SecretPath != "" {
		s.Cfg.SecretPath = strings.Trim(req.SecretPath, "/")
	}
	if req.Username != "" {
		s.Cfg.Username = req.Username
	}
	if len(req.NewPassword) >= 8 {
		s.Cfg.PasswordSalt = config.RandString(16)
		s.Cfg.PasswordHash = config.HashPassword(req.NewPassword, s.Cfg.PasswordSalt)
	}
	if req.DefaultLang != "" {
		s.Cfg.DefaultLang = req.DefaultLang
	}
	if req.DefaultTheme != "" {
		s.Cfg.DefaultTheme = req.DefaultTheme
	}
	s.Cfg.Domain = req.Domain
	if req.TelegramToken != "" {
		s.Cfg.TelegramToken = req.TelegramToken
	}
	s.Cfg.TelegramChatID = req.TelegramChatID
	if req.WatchdogSec >= 10 {
		s.Cfg.WatchdogSec = req.WatchdogSec
	}
	_ = config.Save(s.Cfg)
	s.Store.AddAudit(s.Cfg.Username, "update-settings", "panel", fmt.Sprintf("port=%d", s.Cfg.Port))
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleAudits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"audits": s.Store.ListAudits()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
