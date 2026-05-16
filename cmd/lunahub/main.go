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
	"io"
	"log"
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

const (
	defaultConfigPath = "/etc/lunahub/config.json"
	statusActive      = "active"
	statusDisabled    = "disabled"
)

type Config struct {
	Project        string `json:"project"`
	Domain         string `json:"domain"`
	ACMEEmail      string `json:"acme_email"`
	AdminToken     string `json:"admin_token"`
	PanelListen    string `json:"panel_listen"`
	PublicPanelURL string `json:"public_panel_url"`
	Paths          struct {
		DataFile       string `json:"data_file"`
		XrayConfig     string `json:"xray_config"`
		HysteriaConfig string `json:"hysteria_config"`
	} `json:"paths"`
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
	} `json:"hysteria"`
}

type Store struct {
	Version int    `json:"version"`
	Users   []User `json:"users"`
}

type User struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Email             string     `json:"email"`
	Status            string     `json:"status"`
	VLESSUUID         string     `json:"vless_uuid"`
	HysteriaUsername  string     `json:"hysteria_username"`
	HysteriaPassword  string     `json:"hysteria_password"`
	SubscriptionToken string     `json:"subscription_token"`
	TrafficLimitBytes int64      `json:"traffic_limit_bytes"`
	UsedUploadBytes   int64      `json:"used_upload_bytes"`
	UsedDownloadBytes int64      `json:"used_download_bytes"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Note              string     `json:"note"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type PageData struct {
	Config      Config
	Store       Store
	Token       string
	Message     string
	ActiveCount int
	Now         time.Time
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "init-db":
		err = runInitDB()
	case "doctor":
		err = runDoctor()
	case "status":
		err = runStatus()
	case "apply":
		err = runApply(os.Args[2:])
	case "serve":
		err = runServe()
	case "user":
		err = runUser(os.Args[2:])
	case "sub":
		err = runSub(os.Args[2:])
	case "token":
		err = runToken(os.Args[2:])
	case "config":
		err = runConfig(os.Args[2:])
	case "help", "--help", "-h":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command: %s", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`LunaHub CLI

Commands:
  lunahub init-db
  lunahub doctor
  lunahub status
  lunahub apply [--no-restart]
  lunahub serve
  lunahub config show [--redact=false]
  lunahub token rotate
  lunahub user create --name "User" --email user@example.com [--limit-gb 0] [--days 0] [--note "..."]
  lunahub user list
  lunahub user enable --email user@example.com
  lunahub user disable --email user@example.com
  lunahub user delete --email user@example.com
  lunahub user rotate --email user@example.com
  lunahub sub show --email user@example.com
`)
}

func configPath() string {
	if p := strings.TrimSpace(os.Getenv("LUNAHUB_CONFIG")); p != "" {
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
	if cfg.Project == "" {
		cfg.Project = "LunaHub"
	}
	if cfg.PublicPanelURL == "" && cfg.Domain != "" {
		cfg.PublicPanelURL = "http://" + cfg.Domain + ":9443"
	}
	return cfg, validateConfig(cfg)
}

func validateConfig(cfg Config) error {
	required := map[string]string{
		"domain":                   cfg.Domain,
		"acme_email":               cfg.ACMEEmail,
		"admin_token":              cfg.AdminToken,
		"panel_listen":             cfg.PanelListen,
		"paths.data_file":          cfg.Paths.DataFile,
		"paths.xray_config":        cfg.Paths.XrayConfig,
		"paths.hysteria_config":    cfg.Paths.HysteriaConfig,
		"xray.reality_dest":        cfg.Xray.RealityDest,
		"xray.reality_server_name": cfg.Xray.RealityServerName,
		"xray.reality_private_key": cfg.Xray.RealityPrivateKey,
		"xray.reality_public_key":  cfg.Xray.RealityPublicKey,
		"xray.reality_short_id":    cfg.Xray.RealityShortID,
		"hysteria.listen":          cfg.Hysteria.Listen,
		"hysteria.obfs_password":   cfg.Hysteria.ObfsPassword,
		"hysteria.masquerade_url":  cfg.Hysteria.MasqueradeURL,
	}
	for k, v := range required {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("config field %s is empty", k)
		}
	}
	if cfg.Xray.VLESSPort < 1 || cfg.Xray.VLESSPort > 65535 {
		return fmt.Errorf("xray.vless_port is invalid: %d", cfg.Xray.VLESSPort)
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
	if err := os.MkdirAll(filepath.Dir(cfg.Paths.DataFile), 0750); err != nil {
		return err
	}
	st.Version = 1
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := cfg.Paths.DataFile + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0660); err != nil {
		return err
	}
	return os.Rename(tmp, cfg.Paths.DataFile)
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

func runUser(args []string) error {
	if len(args) == 0 {
		return errors.New("missing user subcommand")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ExitOnError)
		name := fs.String("name", "", "display name")
		email := fs.String("email", "", "email")
		limitGB := fs.Int64("limit-gb", 0, "traffic limit in GB, 0 means unlimited")
		days := fs.Int("days", 0, "validity in days, 0 means no expiration")
		note := fs.String("note", "", "admin note")
		_ = fs.Parse(args[1:])
		_, err := createUser(*name, *email, *limitGB, *days, *note)
		return err
	case "list":
		return listUsers()
	case "enable":
		return userByEmailFlag(args[1:], func(email string) error { return setUserStatus(email, statusActive) })
	case "disable":
		return userByEmailFlag(args[1:], func(email string) error { return setUserStatus(email, statusDisabled) })
	case "delete":
		return userByEmailFlag(args[1:], deleteUser)
	case "rotate":
		return userByEmailFlag(args[1:], rotateUserSecrets)
	default:
		return fmt.Errorf("unknown user subcommand: %s", args[0])
	}
}

func userByEmailFlag(args []string, fn func(string) error) error {
	fs := flag.NewFlagSet("user", flag.ExitOnError)
	email := fs.String("email", "", "email")
	_ = fs.Parse(args)
	return fn(*email)
}

func createUser(name, email string, limitGB int64, days int, note string) (User, error) {
	var zero User
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || email == "" {
		return zero, errors.New("--name and --email are required")
	}
	if !strings.Contains(email, "@") {
		return zero, fmt.Errorf("email looks invalid: %s", email)
	}

	cfg, err := loadConfig()
	if err != nil {
		return zero, err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return zero, err
	}
	for _, u := range st.Users {
		if strings.EqualFold(u.Email, email) {
			return zero, fmt.Errorf("user already exists: %s", email)
		}
	}

	now := time.Now().UTC()
	var expires *time.Time
	if days > 0 {
		t := now.Add(time.Duration(days) * 24 * time.Hour)
		expires = &t
	}
	u := User{
		ID:                randomHex(16),
		Name:              name,
		Email:             email,
		Status:            statusActive,
		VLESSUUID:         newUUID(),
		HysteriaUsername:  safeUsername(email),
		HysteriaPassword:  randomToken(24),
		SubscriptionToken: randomToken(32),
		TrafficLimitBytes: gbToBytes(limitGB),
		ExpiresAt:         expires,
		Note:              strings.TrimSpace(note),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	st.Users = append(st.Users, u)
	if err := saveStore(cfg, st); err != nil {
		return zero, err
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
	sort.Slice(st.Users, func(i, j int) bool { return st.Users[i].Email < st.Users[j].Email })
	fmt.Printf("%-22s %-32s %-10s %-12s %-20s\n", "NAME", "EMAIL", "STATUS", "LIMIT", "EXPIRES")
	for _, u := range st.Users {
		exp := "-"
		if u.ExpiresAt != nil {
			exp = u.ExpiresAt.Format("2006-01-02")
		}
		fmt.Printf("%-22s %-32s %-10s %-12s %-20s\n", clip(u.Name, 22), clip(u.Email, 32), u.Status, formatLimit(u.TrafficLimitBytes), exp)
	}
	return nil
}

func setUserStatus(email, status string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("--email is required")
	}
	cfg, st, idx, err := findUser(email)
	if err != nil {
		return err
	}
	st.Users[idx].Status = status
	st.Users[idx].UpdatedAt = time.Now().UTC()
	if err := saveStore(cfg, st); err != nil {
		return err
	}
	fmt.Printf("%s -> %s\n", email, status)
	return nil
}

func deleteUser(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("--email is required")
	}
	cfg, st, idx, err := findUser(email)
	if err != nil {
		return err
	}
	st.Users = append(st.Users[:idx], st.Users[idx+1:]...)
	if err := saveStore(cfg, st); err != nil {
		return err
	}
	fmt.Println("deleted:", email)
	return nil
}

func rotateUserSecrets(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("--email is required")
	}
	cfg, st, idx, err := findUser(email)
	if err != nil {
		return err
	}
	st.Users[idx].VLESSUUID = newUUID()
	st.Users[idx].HysteriaPassword = randomToken(24)
	st.Users[idx].SubscriptionToken = randomToken(32)
	st.Users[idx].UpdatedAt = time.Now().UTC()
	if err := saveStore(cfg, st); err != nil {
		return err
	}
	fmt.Println("rotated:", email)
	fmt.Println("subscription:", subscriptionURL(cfg, st.Users[idx]))
	return nil
}

func findUser(email string) (Config, Store, int, error) {
	cfg, err := loadConfig()
	if err != nil {
		return cfg, Store{}, -1, err
	}
	st, err := loadStore(cfg)
	if err != nil {
		return cfg, st, -1, err
	}
	for i := range st.Users {
		if strings.EqualFold(st.Users[i].Email, email) {
			return cfg, st, i, nil
		}
	}
	return cfg, st, -1, fmt.Errorf("user not found: %s", email)
}

func runSub(args []string) error {
	if len(args) == 0 {
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
	cfg, st, idx, err := findUser(email)
	if err != nil {
		return err
	}
	u := st.Users[idx]
	fmt.Println("Subscription:")
	fmt.Println(subscriptionURL(cfg, u))
	fmt.Println()
	fmt.Println("VLESS REALITY:")
	fmt.Println(vlessLink(cfg, u))
	fmt.Println()
	fmt.Println("Hysteria2:")
	fmt.Println(hysteriaLink(cfg, u))
	return nil
}

func runToken(args []string) error {
	if len(args) == 0 {
		return errors.New("missing token subcommand")
	}
	switch args[0] {
	case "rotate":
		return rotateAdminToken()
	default:
		return fmt.Errorf("unknown token subcommand: %s", args[0])
	}
}

func rotateAdminToken() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(configPath())
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
	if err := os.WriteFile(configPath(), append(out, '\n'), 0640); err != nil {
		return err
	}
	cfg.AdminToken = raw["admin_token"].(string)
	fmt.Println("new panel URL:", cfg.PublicPanelURL+"/?token="+cfg.AdminToken)
	return nil
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return errors.New("missing config subcommand")
	}
	switch args[0] {
	case "show":
		fs := flag.NewFlagSet("config show", flag.ExitOnError)
		redact := fs.Bool("redact", true, "redact secrets")
		_ = fs.Parse(args[1:])
		return showConfig(*redact)
	default:
		return fmt.Errorf("unknown config subcommand: %s", args[0])
	}
}

func showConfig(redact bool) error {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return err
	}
	if !redact {
		fmt.Print(string(b))
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	redactMap(raw)
	out, _ := json.MarshalIndent(raw, "", "  ")
	fmt.Println(string(out))
	return nil
}

func redactMap(m map[string]any) {
	secretKeys := map[string]bool{
		"admin_token": true, "reality_private_key": true, "reality_public_key": true,
		"reality_short_id": true, "obfs_password": true,
	}
	for k, v := range m {
		if secretKeys[k] {
			m[k] = "REDACTED"
			continue
		}
		if child, ok := v.(map[string]any); ok {
			redactMap(child)
		}
	}
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	noRestart := fs.Bool("no-restart", false, "write configs without restarting services")
	_ = fs.Parse(args)

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
	if *noRestart {
		return nil
	}
	if err := run("systemctl", "restart", "xray.service"); err != nil {
		return fmt.Errorf("restart xray.service failed: %w", err)
	}
	if err := run("systemctl", "enable", "--now", "hysteria-server.service"); err != nil {
		return fmt.Errorf("enable/start hysteria-server.service failed: %w", err)
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
		if isActiveUser(u) {
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
		"inbounds": []any{map[string]any{
			"listen": "0.0.0.0", "port": cfg.Xray.VLESSPort, "protocol": "vless", "tag": "vless-reality-main",
			"settings": map[string]any{"clients": clients, "decryption": "none"},
			"streamSettings": map[string]any{
				"network": "tcp", "security": "reality",
				"realitySettings": map[string]any{
					"show": false, "dest": cfg.Xray.RealityDest, "xver": 0,
					"serverNames": []string{cfg.Xray.RealityServerName},
					"privateKey":  cfg.Xray.RealityPrivateKey,
					"shortIds":    []string{cfg.Xray.RealityShortID},
				},
			},
			"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
		}},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{}}},
	}
	b, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(cfg.Paths.XrayConfig)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "config-*.json")
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
	backupFile(cfg.Paths.XrayConfig)
	if err := os.Rename(tmpPath, cfg.Paths.XrayConfig); err != nil {
		return err
	}
	_ = chownRootGroup(dir, "xray")
	_ = os.Chmod(dir, 0750)
	_ = chownRootGroup(cfg.Paths.XrayConfig, "xray")
	_ = os.Chmod(cfg.Paths.XrayConfig, 0640)
	return nil
}

func writeHysteriaConfig(cfg Config, st Store) error {
	var sb strings.Builder
	sb.WriteString("listen: " + yamlQuote(cfg.Hysteria.Listen) + "\n\n")
	sb.WriteString("acme:\n")
	sb.WriteString("  domains:\n")
	sb.WriteString("    - " + yamlQuote(cfg.Domain) + "\n")
	sb.WriteString("  email: " + yamlQuote(cfg.ACMEEmail) + "\n")
	sb.WriteString("  ca: letsencrypt\n")
	sb.WriteString("  type: http\n\n")
	sb.WriteString("auth:\n")
	sb.WriteString("  type: userpass\n")
	sb.WriteString("  userpass:\n")
	active := 0
	for _, u := range st.Users {
		if isActiveUser(u) {
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
	dir := filepath.Dir(cfg.Paths.HysteriaConfig)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	backupFile(cfg.Paths.HysteriaConfig)
	tmp, err := os.CreateTemp(dir, "config-*.yaml")
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
	_ = os.Chmod(tmpPath, 0600)
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
		{"config file", []string{"test", "-f", configPath()}},
		{"database file", []string{"test", "-f", cfg.Paths.DataFile}},
		{"xray binary", []string{"which", "xray"}},
		{"hysteria binary", []string{"which", "hysteria"}},
		{"lunahub service", []string{"systemctl", "is-enabled", "lunahub.service"}},
		{"xray service", []string{"systemctl", "is-enabled", "xray.service"}},
		{"hysteria service", []string{"systemctl", "is-enabled", "hysteria-server.service"}},
	}
	for _, c := range checks {
		if err := runQuiet(c.Cmd[0], c.Cmd[1:]...); err != nil {
			fmt.Printf("[FAIL] %s: %v\n", c.Name, err)
		} else {
			fmt.Printf("[OK] %s\n", c.Name)
		}
	}
	st, _ := loadStore(cfg)
	fmt.Println("domain:", cfg.Domain)
	fmt.Println("panel:", cfg.PublicPanelURL+"/?token="+cfg.AdminToken)
	fmt.Println("users:", len(st.Users))
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
		if isActiveUser(u) {
			active++
		}
	}
	fmt.Println("LunaHub")
	fmt.Println("domain:", cfg.Domain)
	fmt.Println("panel:", cfg.PublicPanelURL+"/?token="+cfg.AdminToken)
	fmt.Println("users:", len(st.Users))
	fmt.Println("active:", active)
	fmt.Println("xray config:", cfg.Paths.XrayConfig)
	fmt.Println("hysteria config:", cfg.Paths.HysteriaConfig)
	return nil
}

func runServe() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/sub/", handleSubscription(cfg))
	mux.HandleFunc("/", handleDashboard(cfg))
	mux.HandleFunc("/user/create", handleUserCreate(cfg))
	mux.HandleFunc("/user/toggle", handleUserToggle(cfg))
	mux.HandleFunc("/user/delete", handleUserDelete(cfg))
	mux.HandleFunc("/user/rotate", handleUserRotate(cfg))
	mux.HandleFunc("/apply", handleApply(cfg))
	mux.HandleFunc("/token/rotate", handleTokenRotate(cfg))
	log.Println("LunaHub listening on", cfg.PanelListen)
	return http.ListenAndServe(cfg.PanelListen, mux)
}

func handleDashboard(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminAllowed(cfg, r) {
			unauthorized(w)
			return
		}
		st, err := loadStore(cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sort.Slice(st.Users, func(i, j int) bool { return st.Users[i].Email < st.Users[j].Email })
		active := 0
		for _, u := range st.Users {
			if isActiveUser(u) {
				active++
			}
		}
		data := PageData{Config: cfg, Store: st, Token: tokenFromRequest(r), Message: r.URL.Query().Get("msg"), ActiveCount: active, Now: time.Now().UTC()}
		if data.Token == "" {
			data.Token = cfg.AdminToken
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = dashboardTemplate.Execute(w, data)
	}
}

func handleUserCreate(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustPostAdmin(cfg, w, r) {
			return
		}
		limitGB, _ := strconv.ParseInt(r.FormValue("limit_gb"), 10, 64)
		days, _ := strconv.Atoi(r.FormValue("days"))
		_, err := createUser(r.FormValue("name"), r.FormValue("email"), limitGB, days, r.FormValue("note"))
		if err != nil {
			redirectPanel(w, r, cfg, "Ошибка: "+err.Error())
			return
		}
		_ = runApply([]string{})
		redirectPanel(w, r, cfg, "Пользователь создан, конфиги применены")
	}
}

func handleUserToggle(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustPostAdmin(cfg, w, r) {
			return
		}
		email := r.FormValue("email")
		status := r.FormValue("status")
		if status != statusActive {
			status = statusDisabled
		}
		if err := setUserStatus(email, status); err != nil {
			redirectPanel(w, r, cfg, "Ошибка: "+err.Error())
			return
		}
		_ = runApply([]string{})
		redirectPanel(w, r, cfg, "Статус изменён")
	}
}

func handleUserDelete(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustPostAdmin(cfg, w, r) {
			return
		}
		if err := deleteUser(r.FormValue("email")); err != nil {
			redirectPanel(w, r, cfg, "Ошибка: "+err.Error())
			return
		}
		_ = runApply([]string{})
		redirectPanel(w, r, cfg, "Пользователь удалён")
	}
}

func handleUserRotate(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustPostAdmin(cfg, w, r) {
			return
		}
		if err := rotateUserSecrets(r.FormValue("email")); err != nil {
			redirectPanel(w, r, cfg, "Ошибка: "+err.Error())
			return
		}
		_ = runApply([]string{})
		redirectPanel(w, r, cfg, "Ключи пользователя пересозданы")
	}
}

func handleApply(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustPostAdmin(cfg, w, r) {
			return
		}
		if err := runApply([]string{}); err != nil {
			redirectPanel(w, r, cfg, "Ошибка применения: "+err.Error())
			return
		}
		redirectPanel(w, r, cfg, "Конфиги применены")
	}
}

func handleTokenRotate(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustPostAdmin(cfg, w, r) {
			return
		}
		if err := rotateAdminToken(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func handleSubscription(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/sub/")
		st, err := loadStore(cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, u := range st.Users {
			if isActiveUser(u) && subtle.ConstantTimeCompare([]byte(u.SubscriptionToken), []byte(token)) == 1 {
				links := vlessLink(cfg, u) + "\n" + hysteriaLink(cfg, u) + "\n"
				encoded := base64.StdEncoding.EncodeToString([]byte(links))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(encoded))
				return
			}
		}
		http.Error(w, "subscription not found", http.StatusNotFound)
	}
}

func mustPostAdmin(cfg Config, w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	_ = r.ParseForm()
	if !adminAllowed(cfg, r) {
		unauthorized(w)
		return false
	}
	return true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="LunaHub"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func adminAllowed(cfg Config, r *http.Request) bool {
	provided := tokenFromRequest(r)
	if provided == "" || cfg.AdminToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.AdminToken)) == 1
}

func tokenFromRequest(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	if t := r.FormValue("token"); t != "" {
		return t
	}
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func redirectPanel(w http.ResponseWriter, r *http.Request, cfg Config, msg string) {
	t := tokenFromRequest(r)
	if t == "" {
		t = cfg.AdminToken
	}
	q := url.Values{}
	q.Set("token", t)
	if msg != "" {
		q.Set("msg", msg)
	}
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}

func subscriptionURL(cfg Config, u User) string {
	base := strings.TrimRight(cfg.PublicPanelURL, "/")
	if base == "" {
		base = "http://" + cfg.Domain + ":9443"
	}
	return base + "/sub/" + url.PathEscape(u.SubscriptionToken)
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
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", u.VLESSUUID, cfg.Domain, cfg.Xray.VLESSPort, q.Encode(), url.QueryEscape(name))
}

func hysteriaLink(cfg Config, u User) string {
	q := url.Values{}
	q.Set("sni", cfg.Domain)
	q.Set("obfs", "salamander")
	q.Set("obfs-password", cfg.Hysteria.ObfsPassword)
	name := "LunaHub-" + safeLabel(u.Name) + "-HY2"
	auth := url.UserPassword(u.HysteriaUsername, u.HysteriaPassword).String()
	return fmt.Sprintf("hysteria2://%s@%s:443/?%s#%s", auth, cfg.Domain, q.Encode(), url.QueryEscape(name))
}

func isActiveUser(u User) bool {
	if u.Status != statusActive {
		return false
	}
	if u.ExpiresAt != nil && time.Now().UTC().After(*u.ExpiresAt) {
		return false
	}
	if u.TrafficLimitBytes > 0 && u.UsedUploadBytes+u.UsedDownloadBytes >= u.TrafficLimitBytes {
		return false
	}
	return true
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func lookupGroupID(groupName string) (int, error) {
	out, err := exec.Command("getent", "group", groupName).Output()
	if err != nil {
		return 0, err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(parts) < 3 {
		return 0, fmt.Errorf("unexpected getent output for group %s", groupName)
	}
	return strconv.Atoi(parts[2])
}

func chownRootGroup(path, groupName string) error {
	gid, err := lookupGroupID(groupName)
	if err != nil {
		return err
	}
	return os.Chown(path, 0, gid)
}

func backupFile(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	backup := path + ".bak." + time.Now().UTC().Format("20060102-150405")
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.OpenFile(backup, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, in)
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

func gbToBytes(gb int64) int64 {
	if gb <= 0 {
		return 0
	}
	return gb * 1024 * 1024 * 1024
}

func formatLimit(n int64) string {
	if n <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d GB", n/1024/1024/1024)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(n)
	for _, u := range units {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%.1f PB", f/1024)
}

func clip(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

var dashboardTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{
	"subURL":      subscriptionURL,
	"vless":       vlessLink,
	"hysteria":    hysteriaLink,
	"activeUser":  isActiveUser,
	"formatLimit": formatLimit,
	"formatBytes": formatBytes,
}).Parse(`<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>LunaHub</title>
  <style>
    :root { color-scheme: dark; --bg:#070a12; --card:#101624; --card2:#151d30; --text:#edf2ff; --muted:#98a3b8; --line:#26324a; --accent:#8b5cf6; --danger:#ef4444; --ok:#22c55e; --warn:#f59e0b; }
    * { box-sizing:border-box; }
    body { margin:0; font-family:Inter,Segoe UI,system-ui,sans-serif; background:radial-gradient(circle at top left,#1e1b4b 0,#070a12 40%,#05070d 100%); color:var(--text); }
    .wrap { width:min(1180px, calc(100% - 28px)); margin:28px auto 60px; }
    .top { display:flex; justify-content:space-between; gap:16px; align-items:flex-start; margin-bottom:18px; }
    h1 { margin:0; font-size:34px; letter-spacing:-.04em; }
    .subtitle { color:var(--muted); margin-top:6px; }
    .grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:14px; margin:18px 0; }
    .card { background:linear-gradient(180deg,var(--card),rgba(16,22,36,.82)); border:1px solid var(--line); border-radius:18px; padding:16px; box-shadow:0 18px 60px rgba(0,0,0,.24); }
    .metric { color:var(--muted); font-size:13px; }
    .value { font-size:24px; font-weight:800; margin-top:4px; }
    .actions { display:flex; gap:10px; flex-wrap:wrap; }
    button,.btn { border:0; border-radius:12px; background:var(--accent); color:white; padding:10px 13px; font-weight:700; cursor:pointer; text-decoration:none; display:inline-flex; align-items:center; gap:6px; }
    .btn2 { background:#24314c; }
    .danger { background:var(--danger); }
    .warn { background:var(--warn); color:#111827; }
    input,textarea { width:100%; background:#0a1020; color:var(--text); border:1px solid var(--line); border-radius:12px; padding:10px 12px; outline:none; }
    label { color:var(--muted); font-size:13px; display:block; margin-bottom:6px; }
    .form-grid { display:grid; grid-template-columns:1.1fr 1.2fr .7fr .7fr; gap:10px; align-items:end; }
    table { width:100%; border-collapse:collapse; overflow:hidden; }
    th,td { padding:12px 10px; border-bottom:1px solid var(--line); text-align:left; vertical-align:top; }
    th { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.06em; }
    code { background:#080d1a; border:1px solid var(--line); padding:3px 6px; border-radius:8px; color:#c4b5fd; word-break:break-all; }
    .pill { padding:4px 9px; border-radius:999px; font-size:12px; font-weight:800; display:inline-block; }
    .active { background:rgba(34,197,94,.14); color:#86efac; }
    .disabled { background:rgba(239,68,68,.14); color:#fca5a5; }
    .msg { border:1px solid rgba(139,92,246,.5); background:rgba(139,92,246,.16); padding:12px 14px; border-radius:14px; margin:14px 0; }
    .small { color:var(--muted); font-size:12px; }
    .user-actions { display:flex; gap:7px; flex-wrap:wrap; }
    .copy { max-width:260px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; display:block; }
    @media (max-width:900px){ .grid,.form-grid{grid-template-columns:1fr 1fr} .top{display:block} }
    @media (max-width:620px){ .grid,.form-grid{grid-template-columns:1fr} th:nth-child(4),td:nth-child(4){display:none} }
  </style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <div>
      <h1>LunaHub</h1>
      <div class="subtitle">Панель управления VLESS REALITY и Hysteria2</div>
    </div>
    <div class="actions">
      <form method="post" action="/apply"><input type="hidden" name="token" value="{{.Token}}"><button>Применить конфиги</button></form>
      <form method="post" action="/token/rotate" onsubmit="return confirm('Пересоздать admin token? Старый URL панели перестанет работать.')"><input type="hidden" name="token" value="{{.Token}}"><button class="warn">Сменить token</button></form>
    </div>
  </div>

  {{if .Message}}<div class="msg">{{.Message}}</div>{{end}}

  <div class="grid">
    <div class="card"><div class="metric">Домен</div><div class="value">{{.Config.Domain}}</div></div>
    <div class="card"><div class="metric">Всего пользователей</div><div class="value">{{len .Store.Users}}</div></div>
    <div class="card"><div class="metric">Активных</div><div class="value">{{.ActiveCount}}</div></div>
    <div class="card"><div class="metric">Панель</div><div class="value">9443</div></div>
  </div>

  <div class="card">
    <h2>Создать пользователя</h2>
    <form method="post" action="/user/create">
      <input type="hidden" name="token" value="{{.Token}}">
      <div class="form-grid">
        <div><label>Имя</label><input name="name" placeholder="Ivan" required></div>
        <div><label>Email</label><input name="email" type="email" placeholder="ivan@example.com" required></div>
        <div><label>Лимит, GB</label><input name="limit_gb" type="number" min="0" value="0"></div>
        <div><label>Дней</label><input name="days" type="number" min="0" value="0"></div>
      </div>
      <div style="margin-top:10px"><label>Заметка</label><textarea name="note" rows="2" placeholder="Комментарий для себя"></textarea></div>
      <div style="margin-top:12px"><button>Создать и применить</button></div>
    </form>
  </div>

  <div class="card" style="margin-top:14px">
    <h2>Пользователи</h2>
    <table>
      <thead><tr><th>Пользователь</th><th>Статус</th><th>Лимит</th><th>Subscription</th><th>Действия</th></tr></thead>
      <tbody>
      {{range .Store.Users}}
        <tr>
          <td><strong>{{.Name}}</strong><br><span class="small">{{.Email}}</span>{{if .Note}}<br><span class="small">{{.Note}}</span>{{end}}</td>
          <td>{{if activeUser .}}<span class="pill active">active</span>{{else}}<span class="pill disabled">disabled</span>{{end}}</td>
          <td>{{formatLimit .TrafficLimitBytes}}<br><span class="small">used: {{formatBytes .UsedUploadBytes}} / {{formatBytes .UsedDownloadBytes}}</span></td>
          <td><code class="copy">{{subURL $.Config .}}</code></td>
          <td>
            <div class="user-actions">
              {{if eq .Status "active"}}
              <form method="post" action="/user/toggle"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="email" value="{{.Email}}"><input type="hidden" name="status" value="disabled"><button class="btn2">Откл.</button></form>
              {{else}}
              <form method="post" action="/user/toggle"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="email" value="{{.Email}}"><input type="hidden" name="status" value="active"><button>Вкл.</button></form>
              {{end}}
              <form method="post" action="/user/rotate" onsubmit="return confirm('Пересоздать ключи пользователя?')"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="email" value="{{.Email}}"><button class="btn2">Ключи</button></form>
              <form method="post" action="/user/delete" onsubmit="return confirm('Удалить пользователя?')"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="email" value="{{.Email}}"><button class="danger">Удалить</button></form>
            </div>
          </td>
        </tr>
      {{else}}
        <tr><td colspan="5" class="small">Пользователей пока нет.</td></tr>
      {{end}}
      </tbody>
    </table>
  </div>

  <div class="card" style="margin-top:14px">
    <h2>Пути</h2>
    <p><code>{{.Config.Paths.DataFile}}</code></p>
    <p><code>{{.Config.Paths.XrayConfig}}</code></p>
    <p><code>{{.Config.Paths.HysteriaConfig}}</code></p>
    <p class="small">Не публикуй admin token, UUID, subscription links, Hysteria passwords и REALITY private key.</p>
  </div>
</div>
</body>
</html>`))
