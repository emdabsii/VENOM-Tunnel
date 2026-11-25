package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	BaseDir   = "/etc/venom"
	ConfigFile = "/etc/venom/panel.json"
	StateFile  = "/etc/venom/tunnels.json"
	DBFile     = "/etc/venom/venom.db"
	TunnelsDir = "/etc/venom/tunnels"
	CertDir    = "/etc/venom/certs"
)

type PanelConfig struct {
	Port           int    `json:"port"`
	SecretPath     string `json:"secret_path"`
	Username       string `json:"username"`
	PasswordHash   string `json:"password_hash"`
	PasswordSalt   string `json:"password_salt"`
	JWTSecret      string `json:"jwt_secret"`
	DefaultLang    string `json:"default_lang"`
	DefaultTheme   string `json:"default_theme"`
	Domain         string `json:"domain"`
	TLSCert        string `json:"tls_cert"`
	TLSKey         string `json:"tls_key"`
	TelegramToken  string `json:"telegram_token"`
	TelegramChatID string `json:"telegram_chat_id"`
	WatchdogSec    int    `json:"watchdog_sec"`
}

func RandString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[idx.Int64()]
	}
	return string(b)
}

func RandFreePort5Digit() int {
	for i := 0; i < 60; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(39000))
		port := 20000 + int(n.Int64())
		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err == nil {
			_ = ln.Close()
			return port
		}
	}
	return 48291
}

func HashPassword(password, salt string) string {
	h := sha256.Sum256([]byte(salt + ":" + password + ":venom-v2"))
	return hex.EncodeToString(h[:])
}

func EnsureInitialized() (*PanelConfig, string, error) {
	_ = os.MkdirAll(BaseDir, 0700)
	_ = os.MkdirAll(TunnelsDir, 0700)
	_ = os.MkdirAll(CertDir, 0700)

	if data, err := os.ReadFile(ConfigFile); err == nil {
		var cfg PanelConfig
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.Port >= 1024 {
			if cfg.DefaultLang == "" {
				cfg.DefaultLang = "fa"
			}
			if cfg.DefaultTheme == "" {
				cfg.DefaultTheme = "toxic"
			}
			if cfg.WatchdogSec == 0 {
				cfg.WatchdogSec = 30
			}
			return &cfg, "", nil
		}
	}

	plainPass := RandString(14)
	salt := RandString(16)
	cfg := &PanelConfig{
		Port:         RandFreePort5Digit(),
		SecretPath:   strings.ToLower(RandString(10)),
		Username:     "venom_" + strings.ToLower(RandString(4)),
		PasswordSalt: salt,
		PasswordHash: HashPassword(plainPass, salt),
		JWTSecret:    RandString(48),
		DefaultLang:  "fa",
		DefaultTheme: "toxic",
		WatchdogSec:  30,
	}
	if err := Save(cfg); err != nil {
		return nil, "", err
	}
	return cfg, plainPass, nil
}

func Save(cfg *PanelConfig) error {
	_ = os.MkdirAll(filepath.Dir(ConfigFile), 0700)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile, b, 0600)
}
