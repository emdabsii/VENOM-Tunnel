package tunnel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/venom-tunnel/venom/internal/config"
	"github.com/venom-tunnel/venom/internal/store"
)

var (
	validNameRe  = regexp.MustCompile(`^[a-z0-9_-]{2,28}$`)
	validTokenRe = regexp.MustCompile(`^[A-Za-z0-9]{20,64}$`)
)

type PairingPayload struct {
	Name        string `json:"n"`
	Engine      string `json:"e"`
	Role        string `json:"r"` // inverted role for peer
	RemoteIP    string `json:"rip"`
	ControlPort int    `json:"cp"`
	Ports       string `json:"p"`
	Token       string `json:"t"`
}

func ServiceState(svc string) string {
	out, err := exec.Command("systemctl", "is-active", svc).Output()
	if err != nil {
		s := strings.TrimSpace(string(out))
		if s != "" {
			return s
		}
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func Validate(t *store.TunnelRecord) error {
	if !validNameRe.MatchString(t.Name) {
		return fmt.Errorf("invalid tunnel name: use 2-28 lowercase letters, digits, - or _")
	}
	if t.Role != "iran" && t.Role != "foreign" {
		return fmt.Errorf("role must be iran or foreign")
	}
	if t.ControlPort < 1 || t.ControlPort > 65535 {
		return fmt.Errorf("control_port must be 1..65535")
	}
	if strings.HasPrefix(t.Engine, "backhaul-") || t.Engine == "rathole" || strings.HasPrefix(t.Engine, "gost-") || strings.HasPrefix(t.Engine, "frp-") {
		if t.Token == "" {
			t.Token = config.RandString(40)
		}
		if !validTokenRe.MatchString(t.Token) {
			return fmt.Errorf("token must be 20 to 64 alphanumeric characters")
		}
	}
	return nil
}

func BuildAndApply(t *store.TunnelRecord) error {
	if err := Validate(t); err != nil {
		return err
	}
	if t.ID == "" {
		t.ID = "vnm-" + config.RandString(8)
	}
	if t.ServiceName == "" {
		t.ServiceName = "venom-" + t.Name + ".service"
	}
	if t.ConfigPath == "" {
		t.ConfigPath = filepath.Join(config.TunnelsDir, t.Name+".toml")
	}

	if err := os.MkdirAll(filepath.Dir(t.ConfigPath), 0700); err != nil {
		return err
	}

	var execStart string
	switch {
	case strings.HasPrefix(t.Engine, "backhaul-"):
		tr := strings.TrimPrefix(t.Engine, "backhaul-")
		if err := writeBackhaulConfig(t, tr); err != nil {
			return err
		}
		bin := "/etc/backhaul/backhaul"
		if _, err := os.Stat(bin); err != nil {
			bin = "/usr/local/bin/backhaul"
		}
		execStart = fmt.Sprintf("%s -c %s", bin, t.ConfigPath)
	case t.Engine == "rathole":
		if err := writeRatholeConfig(t); err != nil {
			return err
		}
		execStart = fmt.Sprintf("/usr/local/bin/rathole %s", t.ConfigPath)
	default:
		// Generic script runner for gost/xray-reality/gre/frp
		scriptPath := filepath.Join(config.TunnelsDir, t.Name+".sh")
		if err := writeGenericRunner(t, scriptPath); err != nil {
			return err
		}
		execStart = scriptPath + " start"
	}

	unit := fmt.Sprintf(`[Unit]
Description=Venom Tunnel - %s (%s)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=3
LimitNOFILE=1048576
OOMScoreAdjust=-500

[Install]
WantedBy=multi-user.target
`, t.Name, t.Engine, execStart)

	unitPath := filepath.Join("/etc/systemd/system", t.ServiceName)
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", t.ServiceName).Run()
	return exec.Command("systemctl", "restart", t.ServiceName).Run()
}

func writeBackhaulConfig(t *store.TunnelRecord, tr string) error {
	var sb strings.Builder
	if t.Role == "iran" {
		sb.WriteString("[server]\n")
		sb.WriteString(fmt.Sprintf("bind_addr = \"0.0.0.0:%d\"\n", t.ControlPort))
		sb.WriteString(fmt.Sprintf("transport = \"%s\"\n", tr))
		sb.WriteString(fmt.Sprintf("token = \"%s\"\n", t.Token))
		sb.WriteString("keepalive_period = 75\nnodelay = true\nheartbeat = 40\nchannel_size = 4096\n")
		if strings.Contains(tr, "mux") {
			sb.WriteString("mux_con = 8\nmux_version = 1\nmux_framesize = 32768\nmux_recievebuffer = 4194304\nmux_streambuffer = 65536\n")
		}
		if strings.HasPrefix(tr, "wss") {
			cert := filepath.Join(config.CertDir, t.Name+".crt")
			key := filepath.Join(config.CertDir, t.Name+".key")
			if t.Imported && t.ConfigPath == "/etc/backhaul/conf.toml" {
				cert = "/etc/backhaul/server.crt"
				key = "/etc/backhaul/server.key"
			}
			ensureSelfSignedCert(cert, key)
			sb.WriteString(fmt.Sprintf("tls_cert = \"%s\"\ntls_key = \"%s\"\n", cert, key))
		}
		sb.WriteString("sniffer = false\nweb_port = 0\nlog_level = \"info\"\nports = [\n")
		for _, p := range splitPorts(t.Ports) {
			if strings.Contains(p, "=") || strings.Contains(p, "-") {
				sb.WriteString(fmt.Sprintf("    \"%s\",\n", p))
			} else {
				sb.WriteString(fmt.Sprintf("    \"%s=%s\",\n", p, p))
			}
		}
		sb.WriteString("]\n")
	} else {
		sb.WriteString("[client]\n")
		sb.WriteString(fmt.Sprintf("remote_addr = \"%s:%d\"\n", t.RemoteIP, t.ControlPort))
		if strings.Contains(tr, "ws") {
			sb.WriteString("edge_ip = \"\"\n")
		}
		sb.WriteString(fmt.Sprintf("transport = \"%s\"\n", tr))
		sb.WriteString(fmt.Sprintf("token = \"%s\"\n", t.Token))
		sb.WriteString("connection_pool = 8\naggressive_pool = false\nkeepalive_period = 75\ndial_timeout = 10\nnodelay = true\nretry_interval = 3\n")
		if strings.Contains(tr, "mux") {
			sb.WriteString("mux_version = 1\nmux_framesize = 32768\nmux_recievebuffer = 4194304\nmux_streambuffer = 65536\n")
		}
		sb.WriteString("sniffer = false\nweb_port = 0\nlog_level = \"info\"\n")
	}
	return os.WriteFile(t.ConfigPath, []byte(sb.String()), 0600)
}

func writeRatholeConfig(t *store.TunnelRecord) error {
	var sb strings.Builder
	if t.Role == "iran" {
		sb.WriteString(fmt.Sprintf("[server]\nbind_addr = \"0.0.0.0:%d\"\ndefault_token = \"%s\"\n", t.ControlPort, t.Token))
		for _, p := range splitPorts(t.Ports) {
			sb.WriteString(fmt.Sprintf("[server.services.p%s]\nbind_addr = \"0.0.0.0:%s\"\n", p, p))
		}
	} else {
		sb.WriteString(fmt.Sprintf("[client]\nremote_addr = \"%s:%d\"\ndefault_token = \"%s\"\n", t.RemoteIP, t.ControlPort, t.Token))
		for _, p := range splitPorts(t.Ports) {
			sb.WriteString(fmt.Sprintf("[client.services.p%s]\nlocal_addr = \"127.0.0.1:%s\"\n", p, p))
		}
	}
	return os.WriteFile(t.ConfigPath, []byte(sb.String()), 0600)
}

func writeGenericRunner(t *store.TunnelRecord, path string) error {
	var body string
	if strings.HasPrefix(t.Engine, "gost-") {
		tr := strings.TrimPrefix(t.Engine, "gost-")
		if t.Role == "foreign" {
			body = fmt.Sprintf("exec /usr/local/bin/gost -L \"relay+%s://venom:%s@:%d\"\n", tr, t.Token, t.ControlPort)
		} else {
			var args strings.Builder
			for _, p := range splitPorts(t.Ports) {
				args.WriteString(fmt.Sprintf(" -L tcp://:%s/127.0.0.1:%s", p, p))
			}
			body = fmt.Sprintf("exec /usr/local/bin/gost%s -F \"relay+%s://venom:%s@%s:%d\"\n", args.String(), tr, t.Token, t.RemoteIP, t.ControlPort)
		}
	} else {
		body = "exec sleep infinity\n"
	}
	script := "#!/bin/bash\n# Venom Tunnel runner\n" + body
	return os.WriteFile(path, []byte(script), 0700)
}

func ensureSelfSignedCert(cert, key string) {
	if _, err := os.Stat(cert); err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(cert), 0700)
	_ = exec.Command("openssl", "ecparam", "-name", "secp384r1", "-genkey", "-noout", "-out", key).Run()
	_ = exec.Command("openssl", "req", "-new", "-x509", "-sha384", "-key", key, "-out", cert, "-days", "3650", "-subj", "/CN=cdn.venom-edge.net").Run()
	_ = os.Chmod(key, 0600)
}

func splitPorts(ports string) []string {
	var res []string
	for _, p := range strings.Split(ports, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func Action(svc, act string) error {
	switch act {
	case "start", "stop", "restart":
		return exec.Command("systemctl", act, svc).Run()
	default:
		return fmt.Errorf("invalid action: %s", act)
	}
}

func Remove(t *store.TunnelRecord) error {
	_ = exec.Command("systemctl", "disable", "--now", t.ServiceName).Run()
	if !t.Imported {
		_ = os.Remove(filepath.Join("/etc/systemd/system", t.ServiceName))
		_ = os.Remove(t.ConfigPath)
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
	return nil
}

func Logs(svc string, lines int) string {
	if lines < 10 || lines > 300 {
		lines = 60
	}
	out, err := exec.Command("journalctl", "-u", svc, "-n", fmt.Sprintf("%d", lines), "--no-pager").CombinedOutput()
	if err != nil && len(out) == 0 {
		return "No logs available for " + svc
	}
	return string(out)
}

func MakePairingCode(t store.TunnelRecord) string {
	peerRole := "foreign"
	if t.Role == "foreign" {
		peerRole = "iran"
	}
	p := PairingPayload{
		Name:        t.Name,
		Engine:      t.Engine,
		Role:        peerRole,
		RemoteIP:    t.LocalIP,
		ControlPort: t.ControlPort,
		Ports:       t.Ports,
		Token:       t.Token,
	}
	b, _ := json.Marshal(p)
	return "VENOM://" + base64.RawURLEncoding.EncodeToString(b)
}

func DecodePairingCode(code string) (*PairingPayload, error) {
	code = strings.TrimSpace(strings.TrimPrefix(code, "VENOM://"))
	b, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil {
		return nil, err
	}
	var p PairingPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
