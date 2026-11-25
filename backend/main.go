package main

import (
	"embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/venom-tunnel/venom/internal/api"
	"github.com/venom-tunnel/venom/internal/config"
	"github.com/venom-tunnel/venom/internal/store"
	"github.com/venom-tunnel/venom/internal/tunnel"
)

//go:embed webdist/*
var webDistFS embed.FS

func detectPrimaryIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return addr.IP.String()
		}
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func main() {
	cfg, initialPass, err := config.EnsureInitialized()
	if err != nil {
		log.Fatalf("venom init failed: %v", err)
	}
	serverIP := detectPrimaryIP()

	// CLI subcommands for install.sh and venom terminal menu
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init-info":
			fmt.Printf("URL=http://%s:%d/%s/\nUSER=%s\nPASS=%s\nPORT=%d\nPATH=%s\n",
				serverIP, cfg.Port, cfg.SecretPath, cfg.Username, initialPass, cfg.Port, cfg.SecretPath)
			return
		case "reset-auth":
			newPass := config.RandString(14)
			cfg.PasswordSalt = config.RandString(16)
			cfg.PasswordHash = config.HashPassword(newPass, cfg.PasswordSalt)
			_ = config.Save(cfg)
			fmt.Printf("USER=%s\nPASS=%s\n", cfg.Username, newPass)
			return
		case "pair":
			if len(os.Args) < 3 {
				log.Fatal("usage: venomd pair VENOM://...")
			}
			st := store.New()
			p, err := tunnel.DecodePairingCode(os.Args[2])
			if err != nil {
				log.Fatalf("invalid pairing code: %v", err)
			}
			rec := store.TunnelRecord{
				Name: p.Name, Engine: p.Engine, Role: p.Role,
				LocalIP: serverIP, RemoteIP: p.RemoteIP,
				ControlPort: p.ControlPort, Ports: p.Ports, Token: p.Token, AutoWatchdog: true,
			}
			if err := tunnel.BuildAndApply(&rec); err != nil {
				log.Fatalf("apply failed: %v", err)
			}
			_ = st.UpsertTunnel(rec)
			fmt.Printf("Paired tunnel %s (%s, %s) successfully.\n", rec.Name, rec.Engine, rec.Role)
			return
		}
	}

	st := store.New()
	if count := tunnel.AutoImportExisting(st, serverIP); count > 0 {
		log.Printf("[venom] Auto-imported %d existing tunnel(s) into Web Panel", count)
	}

	srv := &api.Server{
		Cfg:      cfg,
		Store:    st,
		WebFS:    webDistFS,
		ServerIP: serverIP,
	}

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	scheme := "http"
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		scheme = "https"
	}
	log.Printf("[venom] Venom Tunnel v2.0.0 panel listening on %s://%s:%d/%s/",
		scheme, serverIP, cfg.Port, strings.Trim(cfg.SecretPath, "/"))

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		log.Fatal(http.ListenAndServeTLS(addr, cfg.TLSCert, cfg.TLSKey, srv.Handler()))
	} else {
		log.Fatal(http.ListenAndServe(addr, srv.Handler()))
	}
}
