package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultConfigPath = "/etc/lunahub/config.json"

type Config struct {
	PanelDomain   string `json:"panel_domain"`
	VPNDomain     string `json:"vpn_domain"`
	Domain        string `json:"domain,omitempty"`
	ACMEEmail     string `json:"acme_email"`
	AdminToken    string `json:"admin_token"`
	PanelListen   string `json:"panel_listen"`
	PublicBaseURL string `json:"public_base_url"`
	Paths         struct {
		DataFile       string `json:"data_file"`
		XrayConfig     string `json:"xray_config"`
		HysteriaConfig string `json:"hysteria_config"`
	} `json:"paths"`
	TLS struct {
		Fullchain string `json:"fullchain"`
		Privkey   string `json:"privkey"`
	} `json:"tls"`
	Xray struct {
		VLESSPort         int    `json:"vless_port"`
		RealityDest       string `json:"reality_dest"`
		RealityServerName string `json:"reality_server_name"`
		RealityPrivateKey string `json:"reality_private_key"`
		RealityPublicKey  string `json:"reality_public_key"`
		RealityShortID    string `json:"reality_short_id"`
	} `json:"xray"`
	Hysteria struct {
		Listen        string `json:"listen"`
		ObfsPassword  string `json:"obfs_password"`
		MasqueradeURL string `json:"masquerade_url"`
		CertFile      string `json:"cert_file"`
		KeyFile       string `json:"key_file"`
	} `json:"hysteria"`
}

type Store struct {
	Version int    `json:"version"`
	Users   []User `json:"users"`
}

type User struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	Status            string    `json:"status"`
	VLESSUUID         string    `json:"vless_uuid"`
	HysteriaUsername  string    `json:"hysteria_username"`
	HysteriaPassword  string    `json:"hysteria_password"`
	SubscriptionToken string    `json:"subscription_token"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type flash struct {
	Kind string
	Text string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init-db":
		must(runInitDB())
	case "doctor":
		must(runDoctor())
	case "apply":
		must(runApply(true))
	case "serve":
		must(runServe())
	case "status":
		must(runStatus())
	case "token":
		must(runToken(os.Args[2:]))
	case "user":
		must(runUser(os.Args[2:]))
	case "sub":
		must(runSub(os.Args[2:]))
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`LunaHub CLI

Commands:
  lunahub init-db
  lunahub doctor
  lunahub status
  lunahub apply
  lunahub serve
  lunahub token rotate
  lunahub user create --name "User" --email user@example.com
  lunahub user list
  lunahub user disable --email user@example.com
  lunahub user enable --email user@example.com
  lunahub user rotate --email user@example.com
  lunahub user delete --email user@example.com
  lunahub sub show --email user@example.com
`)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func configPath() string {
	if p := os.Getenv("LUNAHUB_CONFIG"); p != "" {
		return p
	}
	return defaultConfigPath
}

func loadConfig() (Config, error) {
	var cfg Config

	b, err := os.ReadFile(configPath())
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}

	if cfg.PanelDomain == "" && cfg.Domain != "" {
		cfg.PanelDomain = cfg.Domain
	}
	if cfg.VPNDomain == "" && cfg.Domain != "" {
		cfg.VPNDomain = cfg.Domain
	}
	if cfg.PublicBaseURL == "" && cfg.PanelDomain != "" {
		cfg.PublicBaseURL = "https://" + cfg.PanelDomain
	}
	if cfg.Hysteria.CertFile == "" {
		cfg.Hysteria.CertFile = cfg.TLS.Fullchain
	}
	if cfg.Hysteria.KeyFile == "" {
		cfg.Hysteria.KeyFile = cfg.TLS.Privkey
	}

	if err := validateConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func validateConfig(cfg Config) error {
	required := map[string]string{
		"panel_domain":             cfg.PanelDomain,
		"vpn_domain":               cfg.VPNDomain,
		"acme_email":               cfg.ACMEEmail,
		"admin_token":              cfg.AdminToken,
		"panel_listen":             cfg.PanelListen,
		"public_base_url":          cfg.PublicBaseURL,
		"paths.data_file":          cfg.Paths.DataFile,
		"paths.xray_config":        cfg.Paths.XrayConfig,
		"paths.hysteria_config":    cfg.Paths.HysteriaConfig,
		"tls.fullchain":            cfg.TLS.Fullchain,
		"tls.privkey":              cfg.TLS.Privkey,
		"xray.reality_dest":        cfg.Xray.RealityDest,
		"xray.reality_server_name": cfg.Xray.RealityServerName,
		"xray.reality_private_key": cfg.Xray.RealityPrivateKey,
		"xray.reality_public_key":  cfg.Xray.RealityPublicKey,
		"xray.reality_short_id":    cfg.Xray.RealityShortID,
		"hysteria.listen":          cfg.Hysteria.Listen,
		"hysteria.obfs_password":   cfg.Hysteria.ObfsPassword,
		"hysteria.masquerade_url":  cfg.Hysteria.MasqueradeURL,
		"hysteria.cert_file":       cfg.Hysteria.CertFile,
		"hysteria.key_file":        cfg.Hysteria.KeyFile,
	}

	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("config %s is empty", name)
		}
	}
	if cfg.Xray.VLESSPort <= 0 || cfg.Xray.VLESSPort > 65535 {
		return fmt.Errorf("config xray.vless_port is invalid: %d", cfg.Xray.VLESSPort)
	}
	if _, err := hysteriaPort(cfg); err != nil {
		return err
	}
	if !strings.HasPrefix(cfg.PublicBaseURL, "https://") {
		return errors.New("public_base_url must start with https://")
	}
	return nil
}

func loadStore(cfg Config) (Store, error) {
	var st Store

	b, err := os.ReadFile(cfg.Paths.DataFile)
	if errors.Is(err, os.ErrNotExist) {
		return Store{Version: 1, Users: []User{}}, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	if st.Version == 0 {
		st.Version = 1
	}
	if st.Users == nil {
		st.Users = []User{}
	}
	return st, nil
}

func saveStore(cfg Config, st Store) error {
	if err := os.MkdirAll(filepath.Dir(cfg.Paths.DataFile), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.Paths.DataFile, append(b, '\n'), 0600)
}

func runInitDB() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	if err := saveStore(cfg, st); err != nil {
		return err
	}
	fmt.Println("initialized:", cfg.Paths.DataFile)
	return nil
}

func runToken(args []string) error {
	if len(args) < 1 {
		return errors.New("missing token subcommand")
	}
	if args[0] != "rotate" {
		return fmt.Errorf("unknown token subcommand: %s", args[0])
	}

	path := configPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	raw["admin_token"] = randomHex(24)

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0600); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Println("new panel URL:", panelURL(cfg))
	return nil
}

func runUser(args []string) error {
	if len(args) < 1 {
		return errors.New("missing user subcommand")
	}

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ExitOnError)
		name := fs.String("name", "", "display name")
		email := fs.String("email", "", "email")
		_ = fs.Parse(args[1:])
		_, err := createUser(*name, *email)
		return err
	case "list":
		return listUsers()
	case "disable":
		fs := flag.NewFlagSet("user disable", flag.ExitOnError)
		email := fs.String("email", "", "email")
		_ = fs.Parse(args[1:])
		return setUserStatus(*email, "disabled")
	case "enable":
		fs := flag.NewFlagSet("user enable", flag.ExitOnError)
		email := fs.String("email", "", "email")
		_ = fs.Parse(args[1:])
		return setUserStatus(*email, "active")
	case "rotate":
		fs := flag.NewFlagSet("user rotate", flag.ExitOnError)
		email := fs.String("email", "", "email")
		_ = fs.Parse(args[1:])
		return rotateUser(*email)
	case "delete":
		fs := flag.NewFlagSet("user delete", flag.ExitOnError)
		email := fs.String("email", "", "email")
		_ = fs.Parse(args[1:])
		return deleteUser(*email)
	default:
		return fmt.Errorf("unknown user subcommand: %s", args[0])
	}
}

func createUser(name, email string) (User, error) {
	var empty User
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || email == "" {
		return empty, errors.New("--name and --email are required")
	}

	cfg, err := loadConfig()
	if err != nil {
		return empty, err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return empty, err
	}
	for _, u := range st.Users {
		if strings.EqualFold(u.Email, email) {
			return empty, fmt.Errorf("user already exists: %s", email)
		}
	}

	now := time.Now().UTC()
	u := User{
		ID:                randomHex(16),
		Name:              name,
		Email:             email,
		Status:            "active",
		VLESSUUID:         newUUID(),
		HysteriaUsername:  safeUsername(email),
		HysteriaPassword:  randomToken(24),
		SubscriptionToken: randomToken(32),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	st.Users = append(st.Users, u)
	if err := saveStore(cfg, st); err != nil {
		return empty, err
	}
	fmt.Println("created user:", u.Email)
	fmt.Println("subscription:", subscriptionURL(cfg, u))
	return u, nil
}

func listUsers() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	sortUsers(st.Users)

	fmt.Printf("%-24s %-34s %-10s %-36s %-s\n", "NAME", "EMAIL", "STATUS", "VLESS_UUID", "SUBSCRIPTION")
	for _, u := range st.Users {
		fmt.Printf("%-24s %-34s %-10s %-36s %-s\n", u.Name, u.Email, u.Status, u.VLESSUUID, subscriptionURL(cfg, u))
	}
	return nil
}

func setUserStatus(email, status string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("--email is required")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	for i := range st.Users {
		if strings.EqualFold(st.Users[i].Email, email) {
			st.Users[i].Status = status
			st.Users[i].UpdatedAt = time.Now().UTC()
			if err := saveStore(cfg, st); err != nil {
				return err
			}
			fmt.Printf("%s -> %s\n", email, status)
			return nil
		}
	}
	return fmt.Errorf("user not found: %s", email)
}

func rotateUser(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("--email is required")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	for i := range st.Users {
		if strings.EqualFold(st.Users[i].Email, email) {
			st.Users[i].VLESSUUID = newUUID()
			st.Users[i].HysteriaPassword = randomToken(24)
			st.Users[i].SubscriptionToken = randomToken(32)
			st.Users[i].UpdatedAt = time.Now().UTC()
			if err := saveStore(cfg, st); err != nil {
				return err
			}
			fmt.Println("rotated:", email)
			fmt.Println("subscription:", subscriptionURL(cfg, st.Users[i]))
			return nil
		}
	}
	return fmt.Errorf("user not found: %s", email)
}

func deleteUser(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("--email is required")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}

	filtered := st.Users[:0]
	deleted := false
	for _, u := range st.Users {
		if strings.EqualFold(u.Email, email) {
			deleted = true
			continue
		}
		filtered = append(filtered, u)
	}
	if !deleted {
		return fmt.Errorf("user not found: %s", email)
	}
	st.Users = filtered
	if err := saveStore(cfg, st); err != nil {
		return err
	}
	fmt.Println("deleted:", email)
	return nil
}

func runSub(args []string) error {
	if len(args) < 1 {
		return errors.New("missing sub subcommand")
	}
	switch args[0] {
	case "show":
		fs := flag.NewFlagSet("sub show", flag.ExitOnError)
		email := fs.String("email", "", "email")
		_ = fs.Parse(args[1:])
		return showSub(*email)
	default:
		return fmt.Errorf("unknown sub subcommand: %s", args[0])
	}
}

func showSub(email string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	for _, u := range st.Users {
		if strings.EqualFold(u.Email, email) {
			fmt.Println(subscriptionURL(cfg, u))
			fmt.Println()
			fmt.Println("VLESS REALITY:")
			fmt.Println(vlessLink(cfg, u))
			fmt.Println()
			fmt.Println("Hysteria2:")
			fmt.Println(hysteriaLink(cfg, u))
			return nil
		}
	}
	return fmt.Errorf("user not found: %s", email)
}

func runApply(restart bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	if err := writeXrayConfig(cfg, st); err != nil {
		return err
	}
	if err := writeHysteriaConfig(cfg, st); err != nil {
		return err
	}
	fmt.Println("wrote:", cfg.Paths.XrayConfig)
	fmt.Println("wrote:", cfg.Paths.HysteriaConfig)
	if restart {
		if err := run("systemctl", "restart", "xray.service"); err != nil {
			return fmt.Errorf("restart xray.service failed: %w", err)
		}
		if err := run("systemctl", "restart", "hysteria-server.service"); err != nil {
			return fmt.Errorf("restart hysteria-server.service failed: %w", err)
		}
	}
	return nil
}

func writeXrayConfig(cfg Config, st Store) error {
	type Client struct {
		ID    string `json:"id"`
		Flow  string `json:"flow"`
		Email string `json:"email"`
		Level int    `json:"level"`
	}

	clients := []Client{}
	for _, u := range st.Users {
		if u.Status == "active" {
			clients = append(clients, Client{ID: u.VLESSUUID, Flow: "xtls-rprx-vision", Email: u.Email, Level: 0})
		}
	}

	conf := map[string]any{
		"log":   map[string]any{"loglevel": "info", "access": "/var/log/xray/access.log", "error": "/var/log/xray/error.log"},
		"dns":   map[string]any{"servers": []string{"1.1.1.1", "8.8.8.8"}},
		"stats": map[string]any{},
		"policy": map[string]any{
			"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}},
			"system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true},
		},
		"inbounds": []any{
			map[string]any{
				"listen":   "0.0.0.0",
				"port":     cfg.Xray.VLESSPort,
				"protocol": "vless",
				"tag":      "vless-reality-main",
				"settings": map[string]any{"clients": clients, "decryption": "none"},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"show":        false,
						"dest":        cfg.Xray.RealityDest,
						"xver":        0,
						"serverNames": []string{cfg.Xray.RealityServerName},
						"privateKey":  cfg.Xray.RealityPrivateKey,
						"shortIds":    []string{cfg.Xray.RealityShortID},
					},
				},
				"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
			},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{}}},
	}

	b, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}

	xrayDir := filepath.Dir(cfg.Paths.XrayConfig)
	if err := os.MkdirAll(xrayDir, 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(xrayDir, "config-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if commandExists("xray") {
		if err := run("xray", "run", "-test", "-config", tmpPath); err != nil {
			return fmt.Errorf("xray config test failed: %w", err)
		}
	}
	if _, err := os.Stat(cfg.Paths.XrayConfig); err == nil {
		backup := cfg.Paths.XrayConfig + ".bak." + time.Now().UTC().Format("20060102-150405")
		_ = copyFile(cfg.Paths.XrayConfig, backup, 0600)
	}
	if err := os.Rename(tmpPath, cfg.Paths.XrayConfig); err != nil {
		return err
	}
	if err := chownRootGroup(xrayDir, "xray"); err != nil {
		return err
	}
	if err := os.Chmod(xrayDir, 0750); err != nil {
		return err
	}
	if err := chownRootGroup(cfg.Paths.XrayConfig, "xray"); err != nil {
		return err
	}
	return os.Chmod(cfg.Paths.XrayConfig, 0640)
}

func writeHysteriaConfig(cfg Config, st Store) error {
	var sb strings.Builder

	sb.WriteString("listen: " + yamlQuote(cfg.Hysteria.Listen) + "\n\n")
	sb.WriteString("tls:\n")
	sb.WriteString("  cert: " + yamlQuote(cfg.Hysteria.CertFile) + "\n")
	sb.WriteString("  key: " + yamlQuote(cfg.Hysteria.KeyFile) + "\n\n")
	sb.WriteString("auth:\n")
	sb.WriteString("  type: userpass\n")
	sb.WriteString("  userpass:\n")

	active := 0
	for _, u := range st.Users {
		if u.Status == "active" {
			active++
			sb.WriteString("    " + yamlQuote(u.HysteriaUsername) + ": " + yamlQuote(u.HysteriaPassword) + "\n")
		}
	}
	if active == 0 {
		sb.WriteString("    disabled: " + yamlQuote(randomToken(32)) + "\n")
	}

	sb.WriteString("\nobfs:\n")
	sb.WriteString("  type: salamander\n")
	sb.WriteString("  salamander:\n")
	sb.WriteString("    password: " + yamlQuote(cfg.Hysteria.ObfsPassword) + "\n\n")
	sb.WriteString("masquerade:\n")
	sb.WriteString("  type: proxy\n")
	sb.WriteString("  proxy:\n")
	sb.WriteString("    url: " + yamlQuote(cfg.Hysteria.MasqueradeURL) + "\n")
	sb.WriteString("    rewriteHost: true\n")

	hysteriaDir := filepath.Dir(cfg.Paths.HysteriaConfig)
	if err := os.MkdirAll(hysteriaDir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(cfg.Paths.HysteriaConfig); err == nil {
		backup := cfg.Paths.HysteriaConfig + ".bak." + time.Now().UTC().Format("20060102-150405")
		_ = copyFile(cfg.Paths.HysteriaConfig, backup, 0600)
	}
	tmp, err := os.CreateTemp(hysteriaDir, "config-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(sb.String()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, cfg.Paths.HysteriaConfig)
}

func runDoctor() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	checks := []struct {
		Name string
		Cmd  []string
	}{
		{"xray binary", []string{"which", "xray"}},
		{"hysteria binary", []string{"which", "hysteria"}},
		{"caddy binary", []string{"which", "caddy"}},
		{"xray service", []string{"systemctl", "is-active", "xray.service"}},
		{"hysteria service", []string{"systemctl", "is-active", "hysteria-server.service"}},
		{"caddy service", []string{"systemctl", "is-active", "caddy.service"}},
		{"lunahub service", []string{"systemctl", "is-active", "lunahub.service"}},
	}
	for _, c := range checks {
		if err := run(c.Cmd[0], c.Cmd[1:]...); err != nil {
			fmt.Printf("[FAIL] %s: %v\n", c.Name, err)
		} else {
			fmt.Printf("[OK] %s\n", c.Name)
		}
	}
	fmt.Println("panel:", panelURL(cfg))
	fmt.Println("subscription base:", strings.TrimRight(cfg.PublicBaseURL, "/")+"/sub/<token>")
	fmt.Println("vpn domain:", cfg.VPNDomain)
	fmt.Println("vless port:", cfg.Xray.VLESSPort)
	port, _ := hysteriaPort(cfg)
	fmt.Println("hysteria port:", port)
	fmt.Println("data:", cfg.Paths.DataFile)
	return nil
}

func runStatus() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return err
	}
	active := 0
	for _, u := range st.Users {
		if u.Status == "active" {
			active++
		}
	}
	fmt.Println("LunaHub")
	fmt.Println("panel domain:", cfg.PanelDomain)
	fmt.Println("vpn domain:", cfg.VPNDomain)
	fmt.Println("users:", len(st.Users))
	fmt.Println("active:", active)
	fmt.Println("panel:", panelURL(cfg))
	return nil
}

func runServe() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", withLog(func(w http.ResponseWriter, r *http.Request) { handleDashboard(cfg, w, r) }))
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/sub/", withLog(func(w http.ResponseWriter, r *http.Request) { handleSubscription(cfg, w, r) }))
	mux.HandleFunc("/users/create", withLog(func(w http.ResponseWriter, r *http.Request) { handleUserAction(cfg, w, r, "create") }))
	mux.HandleFunc("/users/enable", withLog(func(w http.ResponseWriter, r *http.Request) { handleUserAction(cfg, w, r, "enable") }))
	mux.HandleFunc("/users/disable", withLog(func(w http.ResponseWriter, r *http.Request) { handleUserAction(cfg, w, r, "disable") }))
	mux.HandleFunc("/users/rotate", withLog(func(w http.ResponseWriter, r *http.Request) { handleUserAction(cfg, w, r, "rotate") }))
	mux.HandleFunc("/users/delete", withLog(func(w http.ResponseWriter, r *http.Request) { handleUserAction(cfg, w, r, "delete") }))
	mux.HandleFunc("/apply", withLog(func(w http.ResponseWriter, r *http.Request) { handleApply(cfg, w, r) }))

	log.Println("LunaHub internal HTTP listening on", cfg.PanelListen)
	return http.ListenAndServe(cfg.PanelListen, mux)
}

func withLog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next(w, r)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func handleDashboard(cfg Config, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !adminAllowed(cfg, r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="LunaHub"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	st, err := loadStore(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sortUsers(st.Users)
	active := 0
	for _, u := range st.Users {
		if u.Status == "active" {
			active++
		}
	}
	port, _ := hysteriaPort(cfg)
	data := struct {
		Config       Config
		Store        Store
		Active       int
		PanelURL     string
		AdminToken   string
		Flash        flash
		Now          string
		HysteriaPort int
	}{
		Config:       cfg,
		Store:        st,
		Active:       active,
		PanelURL:     panelURL(cfg),
		AdminToken:   cfg.AdminToken,
		Flash:        parseFlash(r),
		Now:          time.Now().Format("2006-01-02 15:04:05"),
		HysteriaPort: port,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTemplate.Execute(w, data); err != nil {
		log.Println("dashboard template:", err)
	}
}

func handleSubscription(cfg Config, w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/sub/")
	if token == "" {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	st, err := loadStore(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, u := range st.Users {
		if u.Status == "active" && subtle.ConstantTimeCompare([]byte(u.SubscriptionToken), []byte(token)) == 1 {
			links := vlessLink(cfg, u) + "\n" + hysteriaLink(cfg, u) + "\n"
			encoded := base64.StdEncoding.EncodeToString([]byte(links))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=0; expire=0")
			_, _ = w.Write([]byte(encoded))
			return
		}
	}
	http.Error(w, "subscription not found", http.StatusNotFound)
}

func handleUserAction(cfg Config, w http.ResponseWriter, r *http.Request, action string) {
	if !adminAllowed(cfg, r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, cfg, "error", err.Error())
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	var err error
	switch action {
	case "create":
		_, err = createUser(name, email)
	case "enable":
		err = setUserStatus(email, "active")
	case "disable":
		err = setUserStatus(email, "disabled")
	case "rotate":
		err = rotateUser(email)
	case "delete":
		err = deleteUser(email)
	default:
		err = fmt.Errorf("unknown action: %s", action)
	}
	if err != nil {
		redirectFlash(w, r, cfg, "error", err.Error())
		return
	}
	if err := runApply(true); err != nil {
		redirectFlash(w, r, cfg, "error", "user saved, but apply failed: "+err.Error())
		return
	}
	redirectFlash(w, r, cfg, "ok", "changes applied")
}

func handleApply(cfg Config, w http.ResponseWriter, r *http.Request) {
	if !adminAllowed(cfg, r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := runApply(true); err != nil {
		redirectFlash(w, r, cfg, "error", err.Error())
		return
	}
	redirectFlash(w, r, cfg, "ok", "configs regenerated and services restarted")
}

func adminAllowed(cfg Config, r *http.Request) bool {
	provided := r.URL.Query().Get("token")
	if provided == "" {
		provided = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if provided == "" {
		if cookie, err := r.Cookie("lunahub_token"); err == nil {
			provided = cookie.Value
		}
	}
	if provided == "" || cfg.AdminToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.AdminToken)) == 1
}

func parseFlash(r *http.Request) flash {
	return flash{Kind: r.URL.Query().Get("flash"), Text: r.URL.Query().Get("msg")}
}

func redirectFlash(w http.ResponseWriter, r *http.Request, cfg Config, kind, text string) {
	q := url.Values{}
	q.Set("token", cfg.AdminToken)
	q.Set("flash", kind)
	q.Set("msg", text)
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}

func panelURL(cfg Config) string {
	return strings.TrimRight(cfg.PublicBaseURL, "/") + "/?token=" + url.QueryEscape(cfg.AdminToken)
}

func subscriptionURL(cfg Config, u User) string {
	return strings.TrimRight(cfg.PublicBaseURL, "/") + "/sub/" + url.PathEscape(u.SubscriptionToken)
}

func vlessLink(cfg Config, u User) string {
	q := url.Values{}
	q.Set("encryption", "none")
	q.Set("security", "reality")
	q.Set("sni", cfg.Xray.RealityServerName)
	q.Set("fp", "chrome")
	q.Set("pbk", cfg.Xray.RealityPublicKey)
	q.Set("sid", cfg.Xray.RealityShortID)
	q.Set("type", "tcp")
	q.Set("flow", "xtls-rprx-vision")
	name := "LunaHub-" + safeLabel(u.Name) + "-VLESS"
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", u.VLESSUUID, cfg.VPNDomain, cfg.Xray.VLESSPort, q.Encode(), url.QueryEscape(name))
}

func hysteriaLink(cfg Config, u User) string {
	q := url.Values{}
	q.Set("sni", cfg.VPNDomain)
	q.Set("obfs", "salamander")
	q.Set("obfs-password", cfg.Hysteria.ObfsPassword)
	name := "LunaHub-" + safeLabel(u.Name) + "-HY2"
	auth := url.UserPassword(u.HysteriaUsername, u.HysteriaPassword).String()
	port, _ := hysteriaPort(cfg)
	return fmt.Sprintf("hysteria2://%s@%s:%d/?%s#%s", auth, cfg.VPNDomain, port, q.Encode(), url.QueryEscape(name))
}

func hysteriaPort(cfg Config) (int, error) {
	listen := strings.TrimSpace(cfg.Hysteria.Listen)
	if listen == "" {
		return 0, errors.New("config hysteria.listen is empty")
	}
	if p, err := strconv.Atoi(strings.TrimPrefix(listen, ":")); err == nil {
		if p > 0 && p <= 65535 {
			return p, nil
		}
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return 0, fmt.Errorf("config hysteria.listen is invalid: %s", listen)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("config hysteria.listen port is invalid: %s", port)
	}
	return p, nil
}

var dashboardTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{
	"subURL": subscriptionURL,
	"vless":  vlessLink,
	"hy2":    hysteriaLink,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>LunaHub</title>
  <style>
    :root { color-scheme: dark; --bg:#070a12; --card:#0d1324; --text:#eef3ff; --muted:#8f9ab3; --line:#25304a; --ok:#51d88a; --bad:#ff6b6b; --accent:#8b5cf6; --accent2:#22d3ee; }
    * { box-sizing:border-box; }
    body { margin:0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, Segoe UI, sans-serif; background: radial-gradient(circle at 12% 8%, rgba(139,92,246,.28), transparent 32%), radial-gradient(circle at 88% 10%, rgba(34,211,238,.16), transparent 34%), var(--bg); color:var(--text); }
    a { color:#a7d8ff; }
    .wrap { width:min(1180px, calc(100% - 32px)); margin:32px auto 60px; }
    .top { display:flex; justify-content:space-between; gap:16px; align-items:flex-start; margin-bottom:22px; }
    .brand { display:flex; gap:14px; align-items:center; }
    .logo { width:48px; height:48px; border-radius:16px; background:linear-gradient(135deg,var(--accent),var(--accent2)); box-shadow:0 18px 60px rgba(139,92,246,.35); }
    h1 { margin:0; font-size:32px; letter-spacing:-.04em; }
    h2 { margin-top:0; }
    .muted { color:var(--muted); }
    .pill { display:inline-flex; align-items:center; gap:8px; padding:9px 12px; border:1px solid var(--line); border-radius:999px; background:rgba(16,24,45,.74); color:var(--muted); font-size:13px; }
    .grid { display:grid; grid-template-columns: repeat(4, 1fr); gap:14px; margin-bottom:18px; }
    .card { background:linear-gradient(180deg, rgba(255,255,255,.045), rgba(255,255,255,.015)); border:1px solid var(--line); border-radius:24px; padding:18px; box-shadow:0 22px 70px rgba(0,0,0,.26); }
    .stat .label { color:var(--muted); font-size:13px; margin-bottom:8px; }
    .stat .value { font-size:22px; font-weight:800; letter-spacing:-.03em; overflow-wrap:anywhere; }
    .main { display:grid; grid-template-columns: 360px 1fr; gap:18px; }
    input, button { font:inherit; }
    input { width:100%; border:1px solid var(--line); background:#070b16; color:var(--text); border-radius:14px; padding:12px 13px; outline:none; }
    input:focus { border-color:var(--accent2); box-shadow:0 0 0 3px rgba(34,211,238,.12); }
    label { display:block; color:var(--muted); font-size:13px; margin:12px 0 7px; }
    button { border:0; border-radius:14px; padding:11px 14px; cursor:pointer; color:white; background:linear-gradient(135deg,var(--accent),#5b8cff); font-weight:700; }
    button.secondary { background:#172039; color:#dbe7ff; border:1px solid var(--line); }
    button.danger { background:#3a1824; color:#ffb4c0; border:1px solid #673040; }
    .flash { margin:0 0 18px; padding:13px 15px; border-radius:16px; border:1px solid var(--line); background:#10182d; }
    .flash.ok { border-color:rgba(81,216,138,.5); color:#afffd0; }
    .flash.error { border-color:rgba(255,107,107,.55); color:#ffc3c3; }
    .users { display:grid; gap:14px; }
    .user { padding:16px; border:1px solid var(--line); background:rgba(13,19,36,.86); border-radius:22px; }
    .user-head { display:flex; justify-content:space-between; gap:12px; align-items:flex-start; margin-bottom:12px; }
    .name { font-weight:850; font-size:18px; }
    .email { color:var(--muted); font-size:13px; margin-top:3px; }
    .status { padding:6px 10px; border-radius:999px; font-size:12px; font-weight:800; }
    .status.active { color:#afffd0; background:rgba(81,216,138,.12); border:1px solid rgba(81,216,138,.35); }
    .status.disabled { color:#ffb4c0; background:rgba(255,107,107,.1); border:1px solid rgba(255,107,107,.35); }
    .links { display:grid; gap:8px; margin:12px 0; }
    .copyline { display:grid; grid-template-columns: 1fr auto; gap:8px; }
    .copyline input { font-size:13px; }
    .actions { display:flex; flex-wrap:wrap; gap:8px; margin-top:10px; }
    .actions form { margin:0; }
    .small { font-size:12px; color:var(--muted); }
    code { background:#070b16; border:1px solid var(--line); padding:2px 6px; border-radius:8px; }
    @media (max-width: 980px) { .grid { grid-template-columns: repeat(2, 1fr); } .main { grid-template-columns: 1fr; } .top { flex-direction:column; } }
    @media (max-width: 600px) { .grid { grid-template-columns: 1fr; } .copyline { grid-template-columns:1fr; } }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="top">
      <div class="brand">
        <div class="logo"></div>
        <div>
          <h1>LunaHub</h1>
          <div class="muted">HTTPS panel + Xray REALITY + Hysteria2</div>
        </div>
      </div>
      <div class="pill">{{.Now}}</div>
    </div>

    {{if .Flash.Text}}<div class="flash {{.Flash.Kind}}">{{.Flash.Text}}</div>{{end}}

    <div class="grid">
      <div class="card stat"><div class="label">Panel</div><div class="value">{{.Config.PanelDomain}}</div></div>
      <div class="card stat"><div class="label">VPN domain</div><div class="value">{{.Config.VPNDomain}}</div></div>
      <div class="card stat"><div class="label">Users</div><div class="value">{{len .Store.Users}}</div></div>
      <div class="card stat"><div class="label">Active</div><div class="value">{{.Active}}</div></div>
    </div>

    <div class="main">
      <div class="card">
        <h2>Create user</h2>
        <form method="post" action="/users/create?token={{.AdminToken}}">
          <label>Name</label>
          <input name="name" placeholder="Example: Ivan" required>
          <label>Email / login</label>
          <input name="email" placeholder="ivan@example.com" required>
          <div style="height:14px"></div>
          <button type="submit">Create and apply</button>
        </form>
        <hr style="border:0;border-top:1px solid var(--line);margin:20px 0">
        <form method="post" action="/apply?token={{.AdminToken}}">
          <button class="secondary" type="submit">Regenerate configs</button>
        </form>
        <p class="small">Subscription endpoint: <code>{{.Config.PublicBaseURL}}/sub/...</code></p>
        <p class="small">VLESS REALITY TCP port: <code>{{.Config.Xray.VLESSPort}}</code>. Hysteria2 UDP port: <code>{{.HysteriaPort}}</code>.</p>
      </div>

      <div class="users">
        {{range .Store.Users}}
        <div class="user">
          <div class="user-head">
            <div>
              <div class="name">{{.Name}}</div>
              <div class="email">{{.Email}}</div>
            </div>
            <div class="status {{.Status}}">{{.Status}}</div>
          </div>

          <div class="links">
            <div>
              <div class="small">Subscription URL</div>
              <div class="copyline"><input readonly value="{{subURL $.Config .}}"><button class="secondary copy" type="button">Copy</button></div>
            </div>
            <div>
              <div class="small">VLESS REALITY</div>
              <div class="copyline"><input readonly value="{{vless $.Config .}}"><button class="secondary copy" type="button">Copy</button></div>
            </div>
            <div>
              <div class="small">Hysteria2</div>
              <div class="copyline"><input readonly value="{{hy2 $.Config .}}"><button class="secondary copy" type="button">Copy</button></div>
            </div>
          </div>

          <div class="actions">
            {{if eq .Status "active"}}
            <form method="post" action="/users/disable?token={{$.AdminToken}}"><input type="hidden" name="email" value="{{.Email}}"><button class="secondary">Disable</button></form>
            {{else}}
            <form method="post" action="/users/enable?token={{$.AdminToken}}"><input type="hidden" name="email" value="{{.Email}}"><button class="secondary">Enable</button></form>
            {{end}}
            <form method="post" action="/users/rotate?token={{$.AdminToken}}"><input type="hidden" name="email" value="{{.Email}}"><button class="secondary">Rotate keys</button></form>
            <form method="post" action="/users/delete?token={{$.AdminToken}}" onsubmit="return confirm('Delete user {{.Email}}?')"><input type="hidden" name="email" value="{{.Email}}"><button class="danger">Delete</button></form>
          </div>
        </div>
        {{else}}
        <div class="card"><p class="muted">No users yet. Create the first user on the left.</p></div>
        {{end}}
      </div>
    </div>
  </div>
  <script>
    document.querySelectorAll('.copy').forEach(function(btn) {
      btn.addEventListener('click', async function() {
        const input = btn.parentElement.querySelector('input');
        try {
          await navigator.clipboard.writeText(input.value);
          const old = btn.textContent;
          btn.textContent = 'Copied';
          setTimeout(() => btn.textContent = old, 1200);
        } catch (e) {
          input.select();
          document.execCommand('copy');
        }
      });
    });
  </script>
</body>
</html>`))

func sortUsers(users []User) {
	sort.Slice(users, func(i, j int) bool {
		return strings.ToLower(users[i].Email) < strings.ToLower(users[j].Email)
	})
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func lookupGroupID(groupName string) (int, error) {
	out, err := exec.Command("getent", "group", groupName).Output()
	if err != nil {
		return 0, fmt.Errorf("lookup group %s: %w", groupName, err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(parts) < 3 {
		return 0, fmt.Errorf("lookup group %s: unexpected getent output %q", groupName, strings.TrimSpace(string(out)))
	}
	gid, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("parse gid for group %s: %w", groupName, err)
	}
	return gid, nil
}

func chownRootGroup(path, groupName string) error {
	gid, err := lookupGroupID(groupName)
	if err != nil {
		return err
	}
	if err := os.Chown(path, 0, gid); err != nil {
		return fmt.Errorf("chown root:%s %s: %w", groupName, path, err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, mode)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b), "=")
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func safeUsername(email string) string {
	s := strings.ToLower(email)
	replacer := strings.NewReplacer("@", "_", ".", "_", "+", "_", "-", "_")
	s = replacer.Replace(s)
	var out strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "user_" + randomHex(4)
	}
	return out.String()
}

func safeLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "User"
	}
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func yamlQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
