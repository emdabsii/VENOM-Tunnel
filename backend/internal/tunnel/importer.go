package tunnel

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/venom-tunnel/venom/internal/config"
	"github.com/venom-tunnel/venom/internal/store"
)

// AutoImportExisting discovers already-running Backhaul (/etc/backhaul/conf.toml)
// and Em4dloverS (/etc/emadlovers/tunnels.d/*.conf) tunnels without interrupting them.
func AutoImportExisting(st *store.Store, localIP string) int {
	imported := 0

	// 1. Import /etc/backhaul/conf.toml (e.g. Backhaul v0.7.2 WSSMux)
	if b, err := os.ReadFile("/etc/backhaul/conf.toml"); err == nil {
		if _, exists := st.GetTunnel("backhaul-main"); !exists {
			rec := parseBackhaulTOML(string(b), localIP)
			if rec != nil {
				if err := st.UpsertTunnel(*rec); err == nil {
					st.AddAudit("system", "auto-import", rec.Name, "Imported /etc/backhaul/conf.toml")
					imported++
				}
			}
		}
	}

	// 2. Import legacy /etc/emadlovers/tunnels.d/*.conf
	files, _ := filepath.Glob("/etc/emadlovers/tunnels.d/*.conf")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		kv := parseKV(string(b))
		name := kv["NAME"]
		if name == "" {
			continue
		}
		if _, exists := st.GetTunnel(name); exists {
			continue
		}
		cport, _ := strconv.Atoi(kv["CPORT"])
		rec := store.TunnelRecord{
			ID:           "imp-" + name,
			Name:         name,
			Engine:       kv["TYPE"],
			Role:         kv["ROLE"],
			LocalIP:      kv["LOCAL"],
			RemoteIP:     kv["REMOTE"],
			ControlPort:  cport,
			Ports:        kv["PORTS"],
			Token:        kv["TOKEN"],
			Extra:        map[string]string{"source": "emadlovers"},
			ServiceName:  "emad-" + name + ".service",
			ConfigPath:   f,
			Imported:     true,
			AutoWatchdog: true,
		}
		if err := st.UpsertTunnel(rec); err == nil {
			st.AddAudit("system", "auto-import", name, "Imported from "+f)
			imported++
		}
	}

	return imported
}

func parseBackhaulTOML(txt, localIP string) *store.TunnelRecord {
	role := "iran"
	if strings.Contains(txt, "[client]") {
		role = "foreign"
	}

	getStr := func(key string) string {
		re := regexp.MustCompile(`(?m)^\s*` + key + `\s*=\s*"([^"]*)"`)
		if m := re.FindStringSubmatch(txt); len(m) > 1 {
			return m[1]
		}
		return ""
	}

	tr := getStr("transport")
	if tr == "" {
		tr = "wssmux"
	}
	token := getStr("token")
	cport := 443
	remoteIP := ""

	if role == "iran" {
		addr := getStr("bind_addr")
		if idx := strings.LastIndex(addr, ":"); idx != -1 {
			if p, err := strconv.Atoi(addr[idx+1:]); err == nil {
				cport = p
			}
		}
	} else {
		addr := getStr("remote_addr")
		if idx := strings.LastIndex(addr, ":"); idx != -1 {
			remoteIP = addr[:idx]
			if p, err := strconv.Atoi(addr[idx+1:]); err == nil {
				cport = p
			}
		}
	}

	// Extract ports array entries like "8074=8074" or "443"
	var ports []string
	rePort := regexp.MustCompile(`"(\d+(?:-\d+)?)(?:=\d+)?"`)
	if pIdx := strings.Index(txt, "ports"); pIdx != -1 {
		for _, m := range rePort.FindAllStringSubmatch(txt[pIdx:], -1) {
			if len(m) > 1 {
				ports = append(ports, m[1])
			}
		}
	}

	return &store.TunnelRecord{
		ID:           "bh-" + config.RandString(6),
		Name:         "backhaul-main",
		Engine:       "backhaul-" + tr,
		Role:         role,
		LocalIP:      localIP,
		RemoteIP:     remoteIP,
		ControlPort:  cport,
		Ports:        strings.Join(ports, ","),
		Token:        token,
		Extra:        map[string]string{"version": "0.7.2", "mux_con": "8", "source": "auto-imported"},
		ServiceName:  "backhaul.service",
		ConfigPath:   "/etc/backhaul/conf.toml",
		Imported:     true,
		AutoWatchdog: true,
	}
}

func parseKV(txt string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(txt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return m
}
