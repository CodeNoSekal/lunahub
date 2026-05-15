package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultConfigPath = "/etc/lunahub/config.json"

// Config is intentionally JSON-only at the first stage so the project can be built
// with the Go standard library and no external dependencies.
type Config struct {
	Domain      string `json:"domain"`
	ACMEEmail   string `json:"acme_email"`
	AdminToken  string `json:"admin_token"`
	PanelListen string `json:"panel_listen"`
	Paths       struct {
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

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "init-db":
		must(runInitDB())
	case "doctor":
		must(runDoctor())
	case "apply":
		must(runApply())
	case "serve":
		must(runServe())
	case "status":
		must(runStatus())
	case "user":
		must(runUser(os.Args[2:]))
	case "sub":
		must(runSub(os.Args[2:]))
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`LunaHub CLI

Commands:
  lunahub init-db
  lunahub doctor
  lunahub status
  lunahub apply
  lunahub serve
  lunahub user create --name "User" --email user@example.com
  lunahub user list
  lunahub user disable --email user@example.com
  lunahub user enable --email user@example.com
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
	if cfg.Paths.DataFile == "" {
		return cfg, errors.New("config paths.data_file is empty")
	}
	return cfg, nil
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
		return createUser(*name, *email)
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
	default:
		return fmt.Errorf("unknown user subcommand: %s", args[0])
	}
}

func createUser(name, email string) error {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || email == "" {
		return errors.New("--name and --email are required")
	}
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
			return fmt.Errorf("user already exists: %s", email)
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
		return err
	}
	fmt.Println("created user:", u.Email)
	fmt.Println("subscription:", subscriptionURL(cfg, u))
	return nil
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
	fmt.Printf("%-24s %-30s %-10s %-36s\n", "NAME", "EMAIL", "STATUS", "VLESS_UUID")
	for _, u := range st.Users {
		fmt.Printf("%-24s %-30s %-10s %-36s\n", u.Name, u.Email, u.Status, u.VLESSUUID)
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
			fmt.Println("VLESS:")
			fmt.Println(vlessLink(cfg, u))
			fmt.Println()
			fmt.Println("Hysteria2:")
			fmt.Println(hysteriaLink(cfg, u))
			return nil
		}
	}
	return fmt.Errorf("user not found: %s", email)
}

func runApply() error {
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
	_ = run("systemctl", "restart", "xray.service")
	_ = run("systemctl", "restart", "hysteria-server.service")
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
		"log": map[string]any{
			"loglevel": "info",
			"access":   "/var/log/xray/access.log",
			"error":    "/var/log/xray/error.log",
		},
		"dns":   map[string]any{"servers": []string{"1.1.1.1", "8.8.8.8"}},
		"stats": map[string]any{},
		"policy": map[string]any{
			"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true}},
			"system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true},
		},
		"inbounds": []any{
			map[string]any{
				"listen":   "0.0.0.0",
				"port":     cfg.Xray.VLESSPort,
				"protocol": "vless",
				"tag":      "vless-reality-main",
				"settings": map[string]any{
					"clients":    clients,
					"decryption": "none",
				},
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
	if err := os.MkdirAll(filepath.Dir(cfg.Paths.XrayConfig), 0755); err != nil {
		return err
	}
	return os.WriteFile(cfg.Paths.XrayConfig, append(b, '\n'), 0644)
}

func writeHysteriaConfig(cfg Config, st Store) error {
	var sb strings.Builder
	sb.WriteString("listen: " + cfg.Hysteria.Listen + "\n\n")
	sb.WriteString("acme:\n")
	sb.WriteString("  domains:\n")
	sb.WriteString("    - " + cfg.Domain + "\n")
	sb.WriteString("  email: " + cfg.ACMEEmail + "\n")
	sb.WriteString("  ca: letsencrypt\n")
	sb.WriteString("  type: http\n\n")
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
		sb.WriteString("    disabled: disabled\n")
	}
	sb.WriteString("\n")
	sb.WriteString("obfs:\n")
	sb.WriteString("  type: salamander\n")
	sb.WriteString("  salamander:\n")
	sb.WriteString("    password: " + yamlQuote(cfg.Hysteria.ObfsPassword) + "\n\n")
	sb.WriteString("masquerade:\n")
	sb.WriteString("  type: proxy\n")
	sb.WriteString("  proxy:\n")
	sb.WriteString("    url: " + cfg.Hysteria.MasqueradeURL + "\n")
	sb.WriteString("    rewriteHost: true\n")
	if err := os.MkdirAll(filepath.Dir(cfg.Paths.HysteriaConfig), 0755); err != nil {
		return err
	}
	return os.WriteFile(cfg.Paths.HysteriaConfig, []byte(sb.String()), 0644)
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
		{"xray service", []string{"systemctl", "is-enabled", "xray.service"}},
		{"hysteria service", []string{"systemctl", "is-enabled", "hysteria-server.service"}},
		{"lunahub service", []string{"systemctl", "is-enabled", "lunahub.service"}},
	}
	for _, c := range checks {
		if err := run(c.Cmd[0], c.Cmd[1:]...); err != nil {
			fmt.Printf("[FAIL] %s: %v\n", c.Name, err)
		} else {
			fmt.Printf("[OK]   %s\n", c.Name)
		}
	}
	fmt.Println("domain:", cfg.Domain)
	fmt.Println("panel:", "http://"+cfg.Domain+":9443")
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
	fmt.Println("domain:", cfg.Domain)
	fmt.Println("users:", len(st.Users))
	fmt.Println("active:", active)
	fmt.Println("panel:", "http://"+cfg.Domain+":9443")
	return nil
}

func runServe() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		st, _ := loadStore(cfg)
		_ = dashboardTemplate.Execute(w, map[string]any{"Config": cfg, "Store": st})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/sub/", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/sub/")
		st, err := loadStore(cfg)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		for _, u := range st.Users {
			if u.Status == "active" && u.SubscriptionToken == token {
				links := vlessLink(cfg, u) + "\n" + hysteriaLink(cfg, u) + "\n"
				encoded := base64.StdEncoding.EncodeToString([]byte(links))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(encoded))
				return
			}
		}
		http.Error(w, "subscription not found", 404)
	})

	log.Println("LunaHub listening on", cfg.PanelListen)
	return http.ListenAndServe(cfg.PanelListen, mux)
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>LunaHub</title><style>body{font-family:system-ui,sans-serif;max-width:960px;margin:40px auto;padding:0 16px}table{border-collapse:collapse;width:100%}td,th{border-bottom:1px solid #ddd;padding:8px;text-align:left}code{background:#f2f2f2;padding:2px 4px;border-radius:4px}</style></head>
<body>
<h1>LunaHub</h1>
<p>Domain: <code>{{.Config.Domain}}</code></p>
<p>Users: <code>{{len .Store.Users}}</code></p>
<h2>Users</h2>
<table><tr><th>Name</th><th>Email</th><th>Status</th><th>Subscription</th></tr>{{range .Store.Users}}<tr><td>{{.Name}}</td><td>{{.Email}}</td><td>{{.Status}}</td><td><code>/sub/{{.SubscriptionToken}}</code></td></tr>{{end}}</table>
<p>This is a temporary MVP dashboard. Use CLI for management.</p>
</body></html>`))

func subscriptionURL(cfg Config, u User) string {
	return "http://" + cfg.Domain + ":9443/sub/" + url.PathEscape(u.SubscriptionToken)
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
	auth := u.HysteriaUsername + ":" + u.HysteriaPassword
	return fmt.Sprintf("hysteria2://%s@%s:443/?%s#%s", auth, cfg.Domain, q.Encode(), url.QueryEscape(name))
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
