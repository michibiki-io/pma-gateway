package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	defaultPublicBasePath           = "/dbadmin"
	defaultPMAPath                  = "/_pma"
	defaultFrontendPath             = "/_gateway"
	defaultAPIPath                  = "/_api"
	defaultSignonPath               = "/_signon.php"
	defaultTimestampDisplayFormat   = "2006-01-02 15:04:05 MST"
	defaultTimestampDisplayTimeZone = "Asia/Tokyo"
	bootstrapEnvPrefix              = "env:"
	bootstrapSecretPrefix           = "secret:"
)

type Paths struct {
	PublicBasePath string
	PMAPath        string
	FrontendPath   string
	APIPath        string
	SignonPath     string
}

type BootstrapCredential struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DBHost      string `json:"dbHost"`
	DBPort      int    `json:"dbPort"`
	DBUser      string `json:"dbUser"`
	DBPassword  string `json:"dbPassword"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type BootstrapMapping struct {
	SubjectType  string `json:"subjectType"`
	Subject      string `json:"subject"`
	CredentialID string `json:"credentialId"`
}

type BootstrapDocument struct {
	Credentials []BootstrapCredential `json:"credentials"`
	Mappings    []BootstrapMapping    `json:"mappings"`
}

type Bootstrap struct {
	Enabled bool
	Mode    string
	RawJSON string
}

type Config struct {
	ListenAddr             string
	LogLevel               string
	DataDir                string
	DatabaseDriver         string
	DatabasePath           string
	DatabaseDSN            string
	Paths                  Paths
	MasterKey              []byte
	InternalSharedSecret   string
	TicketTTL              time.Duration
	CredentialTestTimeout  time.Duration
	TimestampDisplayFormat string
	TimestampDisplayZone   *time.Location
	UserHeader             string
	GroupsHeader           string
	GroupsSeparator        string
	AdminUsers             map[string]struct{}
	AdminGroups            map[string]struct{}
	AllowedOrigins         []string
	AppCheckMode           string
	AppCheckVerifiedHeader string
	TrustProxyHeaders      bool
	TrustedProxyCIDRs      []*net.IPNet
	Bootstrap              Bootstrap
}

func Load() (Config, error) {
	paths := Paths{
		PublicBasePath: normalizePublicBase(envString("PMA_GATEWAY_PUBLIC_BASE_PATH", defaultPublicBasePath)),
		PMAPath:        normalizeSubpath(envString("PMA_GATEWAY_PMA_PATH", defaultPMAPath)),
		FrontendPath:   normalizeSubpath(envString("PMA_GATEWAY_FRONTEND_PATH", defaultFrontendPath)),
		APIPath:        normalizeSubpath(envString("PMA_GATEWAY_API_PATH", defaultAPIPath)),
		SignonPath:     normalizeSubpath(envString("PMA_GATEWAY_SIGNON_PATH", defaultSignonPath)),
	}
	if err := paths.Validate(); err != nil {
		return Config{}, err
	}

	devInsecure := envBool("PMA_GATEWAY_DEV_INSECURE_EPHEMERAL_KEY", false)
	key, err := loadMasterKey(devInsecure)
	if err != nil {
		return Config{}, err
	}

	internalSecret, err := secretFromEnvOrFile("PMA_GATEWAY_INTERNAL_SHARED_SECRET", "PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE")
	if err != nil {
		return Config{}, err
	}
	if internalSecret == "" {
		if !devInsecure {
			return Config{}, errors.New("PMA_GATEWAY_INTERNAL_SHARED_SECRET or PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE is required")
		}
		internalSecret, err = generateEphemeralSecret()
		if err != nil {
			return Config{}, err
		}
	}

	dataDir := envString("PMA_GATEWAY_DATA_DIR", "/var/lib/pma-gateway")
	databaseDriver := strings.ToLower(strings.TrimSpace(envString("PMA_GATEWAY_DATABASE_DRIVER", "sqlite")))
	if databaseDriver != "sqlite" && databaseDriver != "mysql" {
		return Config{}, fmt.Errorf("unsupported PMA_GATEWAY_DATABASE_DRIVER %q", databaseDriver)
	}
	databaseDSN, err := databaseDSN(databaseDriver)
	if err != nil {
		return Config{}, err
	}
	cidrs, err := parseCIDRs(envString("PMA_GATEWAY_TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128"))
	if err != nil {
		return Config{}, err
	}

	bootstrapRaw, err := bootstrapJSON()
	if err != nil {
		return Config{}, err
	}

	mode := envString("PMA_GATEWAY_BOOTSTRAP_MODE", "first-run")
	if mode != "first-run" && mode != "reconcile" {
		return Config{}, fmt.Errorf("unsupported PMA_GATEWAY_BOOTSTRAP_MODE %q", mode)
	}
	timestampZone, err := loadTimestampDisplayLocation(envString("PMA_GATEWAY_TIMESTAMP_TIMEZONE", defaultTimestampDisplayTimeZone))
	if err != nil {
		return Config{}, fmt.Errorf("invalid PMA_GATEWAY_TIMESTAMP_TIMEZONE: %w", err)
	}

	cfg := Config{
		ListenAddr:             envString("PMA_GATEWAY_LISTEN_ADDR", "127.0.0.1:8080"),
		LogLevel:               envString("PMA_GATEWAY_LOG_LEVEL", "info"),
		DataDir:                dataDir,
		DatabaseDriver:         databaseDriver,
		DatabasePath:           envString("PMA_GATEWAY_DATABASE_PATH", path.Join(dataDir, "pma-gateway.db")),
		DatabaseDSN:            databaseDSN,
		Paths:                  paths,
		MasterKey:              key,
		InternalSharedSecret:   internalSecret,
		TicketTTL:              time.Duration(envInt("PMA_GATEWAY_SIGNON_TICKET_TTL_SECONDS", 60)) * time.Second,
		CredentialTestTimeout:  time.Duration(envInt("PMA_GATEWAY_CREDENTIAL_TEST_TIMEOUT_SECONDS", 10)) * time.Second,
		TimestampDisplayFormat: envString("PMA_GATEWAY_TIMESTAMP_FORMAT", defaultTimestampDisplayFormat),
		TimestampDisplayZone:   timestampZone,
		UserHeader:             envString("PMA_GATEWAY_USER_HEADER", "Remote-User"),
		GroupsHeader:           envString("PMA_GATEWAY_GROUPS_HEADER", "Remote-Groups"),
		GroupsSeparator:        envString("PMA_GATEWAY_GROUPS_SEPARATOR", ","),
		AdminUsers:             parseSet(envString("PMA_GATEWAY_ADMIN_USERS", "")),
		AdminGroups:            parseSet(envString("PMA_GATEWAY_ADMIN_GROUPS", "")),
		AllowedOrigins:         parseList(envString("PMA_GATEWAY_ALLOWED_ORIGINS", "")),
		AppCheckMode:           envString("PMA_GATEWAY_APPCHECK_MODE", "disabled"),
		AppCheckVerifiedHeader: envString("PMA_GATEWAY_APPCHECK_VERIFIED_HEADER", "X-AppCheck-Verified"),
		TrustProxyHeaders:      envBool("PMA_GATEWAY_TRUST_PROXY_HEADERS", true),
		TrustedProxyCIDRs:      cidrs,
		Bootstrap: Bootstrap{
			Enabled: envBool("PMA_GATEWAY_BOOTSTRAP_ENABLED", bootstrapRaw != ""),
			Mode:    mode,
			RawJSON: bootstrapRaw,
		},
	}
	if cfg.TicketTTL <= 0 {
		return Config{}, errors.New("PMA_GATEWAY_SIGNON_TICKET_TTL_SECONDS must be positive")
	}
	if cfg.CredentialTestTimeout <= 0 {
		return Config{}, errors.New("PMA_GATEWAY_CREDENTIAL_TEST_TIMEOUT_SECONDS must be positive")
	}
	if cfg.AppCheckMode != "disabled" && cfg.AppCheckMode != "trusted-header" && cfg.AppCheckMode != "required" {
		return Config{}, fmt.Errorf("unsupported PMA_GATEWAY_APPCHECK_MODE %q", cfg.AppCheckMode)
	}
	return cfg, nil
}

func (p Paths) Validate() error {
	seen := map[string]string{}
	for name, value := range map[string]string{
		"PMA":      p.PMAPath,
		"Frontend": p.FrontendPath,
		"API":      p.APIPath,
		"Signon":   p.SignonPath,
	} {
		full := JoinURLPath(p.PublicBasePath, value)
		if existing := seen[full]; existing != "" {
			return fmt.Errorf("path conflict between %s and %s at %s", existing, name, full)
		}
		seen[full] = name
	}
	return nil
}

func (p Paths) PublicEntryPath() string {
	if p.PublicBasePath == "" {
		return "/"
	}
	return p.PublicBasePath + "/"
}

func (p Paths) PMABasePath() string {
	return ensureTrailingSlash(JoinURLPath(p.PublicBasePath, p.PMAPath))
}

func (p Paths) FrontendBasePath() string {
	return ensureTrailingSlash(JoinURLPath(p.PublicBasePath, p.FrontendPath))
}

func (p Paths) APIBasePath() string {
	return JoinURLPath(p.PublicBasePath, p.APIPath, "/v1")
}

func (p Paths) SignonURL() string {
	return JoinURLPath(p.PublicBasePath, p.SignonPath)
}

func JoinURLPath(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || part == "/" {
			continue
		}
		cleaned = append(cleaned, strings.Trim(part, "/"))
	}
	if len(cleaned) == 0 {
		return "/"
	}
	return "/" + path.Join(cleaned...)
}

func (c Config) IsAdmin(user string, groups []string) bool {
	if _, ok := c.AdminUsers[user]; ok {
		return true
	}
	for _, group := range groups {
		if _, ok := c.AdminGroups[group]; ok {
			return true
		}
	}
	return false
}

func (c Config) IsTrustedRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (c Config) FormatTimestamp(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return value
	}
	return parsed.In(c.timestampDisplayLocation()).Format(c.timestampDisplayFormat())
}

func (c Config) timestampDisplayFormat() string {
	if value := strings.TrimSpace(c.TimestampDisplayFormat); value != "" {
		return value
	}
	return defaultTimestampDisplayFormat
}

func (c Config) timestampDisplayLocation() *time.Location {
	if c.TimestampDisplayZone != nil {
		return c.TimestampDisplayZone
	}
	location, err := loadTimestampDisplayLocation(defaultTimestampDisplayTimeZone)
	if err != nil {
		return time.FixedZone("JST", 9*60*60)
	}
	return location
}

func loadTimestampDisplayLocation(value string) (*time.Location, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		name = defaultTimestampDisplayTimeZone
	}
	if strings.EqualFold(name, "JST") {
		return time.FixedZone("JST", 9*60*60), nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	return location, nil
}

func ParseBootstrapDocument(raw string) (BootstrapDocument, error) {
	var doc BootstrapDocument
	if strings.TrimSpace(raw) == "" {
		return doc, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, err
	}
	for index := range doc.Credentials {
		dbUser, err := resolveBootstrapValue(doc.Credentials[index].DBUser)
		if err != nil {
			return doc, fmt.Errorf("resolve bootstrap credential %q dbUser: %w", doc.Credentials[index].ID, err)
		}
		dbPassword, err := resolveBootstrapValue(doc.Credentials[index].DBPassword)
		if err != nil {
			return doc, fmt.Errorf("resolve bootstrap credential %q dbPassword: %w", doc.Credentials[index].ID, err)
		}
		doc.Credentials[index].DBUser = dbUser
		doc.Credentials[index].DBPassword = dbPassword
	}
	return doc, nil
}

func resolveBootstrapValue(value string) (string, error) {
	switch {
	case strings.HasPrefix(value, bootstrapEnvPrefix):
		name := strings.TrimSpace(strings.TrimPrefix(value, bootstrapEnvPrefix))
		if name == "" {
			return "", errors.New("env reference must not be empty")
		}
		resolved := os.Getenv(name)
		if resolved == "" {
			return "", fmt.Errorf("environment variable %s is empty or unset", name)
		}
		return resolved, nil
	case strings.HasPrefix(value, bootstrapSecretPrefix):
		name := strings.TrimSpace(strings.TrimPrefix(value, bootstrapSecretPrefix))
		if name == "" {
			return "", errors.New("secret reference must not be empty")
		}
		resolved, err := secretFromEnvOrFile(name, name+"_FILE")
		if err != nil {
			return "", err
		}
		if resolved == "" {
			return "", fmt.Errorf("secret %s is empty or unset", name)
		}
		return resolved, nil
	default:
		return value, nil
	}
}

func normalizePublicBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	return normalizeSubpath(value)
}

func normalizeSubpath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return "/"
	}
	return "/" + strings.Trim(path.Clean("/"+value), "/")
}

func ensureTrailingSlash(value string) string {
	if value == "/" || strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range parseList(value) {
		out[item] = struct{}{}
	}
	return out
}

func parseList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseCIDRs(value string) ([]*net.IPNet, error) {
	parts := parseList(value)
	out := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", part, err)
		}
		out = append(out, network)
	}
	return out, nil
}

func secretFromEnvOrFile(valueName, fileName string) (string, error) {
	value := os.Getenv(valueName)
	file := os.Getenv(fileName)
	if value != "" && file != "" {
		return "", fmt.Errorf("set only one of %s or %s", valueName, fileName)
	}
	if value != "" {
		return strings.TrimSpace(value), nil
	}
	if file == "" {
		return "", nil
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func databaseDSN(driver string) (string, error) {
	dsn, err := secretFromEnvOrFile("PMA_GATEWAY_DATABASE_DSN", "PMA_GATEWAY_DATABASE_DSN_FILE")
	if err != nil {
		return "", err
	}
	if dsn != "" || driver != "mysql" {
		return dsn, nil
	}

	password, err := secretFromEnvOrFile("PMA_GATEWAY_MYSQL_PASSWORD", "PMA_GATEWAY_MYSQL_PASSWORD_FILE")
	if err != nil {
		return "", err
	}
	host := envString("PMA_GATEWAY_MYSQL_HOST", "mysql")
	port := envString("PMA_GATEWAY_MYSQL_PORT", "3306")
	user := envString("PMA_GATEWAY_MYSQL_USER", "pma_gateway")
	database := envString("PMA_GATEWAY_MYSQL_DATABASE", "pma_gateway")
	params := envString("PMA_GATEWAY_MYSQL_PARAMS", "charset=utf8mb4&parseTime=false&timeout=5s&readTimeout=10s&writeTimeout=10s")
	if password == "" {
		return "", errors.New("PMA_GATEWAY_DATABASE_DSN or PMA_GATEWAY_MYSQL_PASSWORD is required when PMA_GATEWAY_DATABASE_DRIVER=mysql")
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s", user, password, host, port, database, params), nil
}

func loadMasterKey(devInsecure bool) ([]byte, error) {
	raw, err := secretFromEnvOrFile("PMA_GATEWAY_MASTER_KEY_BASE64", "PMA_GATEWAY_MASTER_KEY_FILE")
	if err != nil {
		return nil, err
	}
	if raw == "" {
		if !devInsecure {
			return nil, errors.New("PMA_GATEWAY_MASTER_KEY_BASE64 or PMA_GATEWAY_MASTER_KEY_FILE is required")
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		return key, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode PMA_GATEWAY_MASTER_KEY_BASE64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

func generateEphemeralSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(secret), nil
}

func bootstrapJSON() (string, error) {
	value := os.Getenv("PMA_GATEWAY_BOOTSTRAP_CONFIG_JSON")
	file := os.Getenv("PMA_GATEWAY_BOOTSTRAP_CONFIG_FILE")
	if value != "" && file != "" {
		return "", errors.New("set only one of PMA_GATEWAY_BOOTSTRAP_CONFIG_JSON or PMA_GATEWAY_BOOTSTRAP_CONFIG_FILE")
	}
	if value != "" {
		return value, nil
	}
	if file == "" {
		return "", nil
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
