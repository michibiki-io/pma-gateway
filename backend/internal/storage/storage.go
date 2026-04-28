package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/michibiki-io/pma-gateway/backend/internal/auditmeta"
	"github.com/michibiki-io/pma-gateway/backend/internal/config"
	pmacrypto "github.com/michibiki-io/pma-gateway/backend/internal/crypto"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthorized    = errors.New("not authorized")
	ErrTicketInvalid   = errors.New("ticket is invalid")
	ErrTicketExpired   = errors.New("ticket is expired")
	ErrTicketUsed      = errors.New("ticket has already been used")
	ErrInvalidArgument = errors.New("invalid argument")
)

type Store struct {
	db      *sql.DB
	cipher  *pmacrypto.Cipher
	dialect string
}

type Options struct {
	Driver string
	Path   string
	DSN    string
}

type Credential struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DBHost      string `json:"dbHost"`
	DBPort      int    `json:"dbPort"`
	DBUser      string `json:"dbUser"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type CredentialInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DBHost      string `json:"dbHost"`
	DBPort      int    `json:"dbPort"`
	DBUser      string `json:"dbUser"`
	DBPassword  string `json:"dbPassword"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type CredentialTestInput struct {
	ExistingCredentialID string
	DBHost               string
	DBPort               int
	DBUser               string
	DBPassword           string
	Timeout              time.Duration
}

type Mapping struct {
	ID           string `json:"id"`
	SubjectType  string `json:"subjectType"`
	Subject      string `json:"subject"`
	CredentialID string `json:"credentialId"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

type MappingInput struct {
	ID           string `json:"id"`
	SubjectType  string `json:"subjectType"`
	Subject      string `json:"subject"`
	CredentialID string `json:"credentialId"`
}

type AuditEvent struct {
	ID            string         `json:"id"`
	Timestamp     string         `json:"timestamp"`
	Actor         string         `json:"actor"`
	ActorGroups   []string       `json:"actorGroups,omitempty"`
	Action        string         `json:"action"`
	TargetType    string         `json:"targetType"`
	TargetID      string         `json:"targetId,omitempty"`
	Result        string         `json:"result"`
	RemoteAddress string         `json:"remoteAddress,omitempty"`
	UserAgent     string         `json:"userAgent,omitempty"`
	Message       string         `json:"message,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type AuditFilter struct {
	Actor      string
	Action     string
	TargetType string
	Result     string
	From       string
	To         string
	Page       int
	PageSize   int
}

type AuditPage struct {
	Items      []AuditEvent `json:"items"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalItems int          `json:"totalItems"`
	TotalPages int          `json:"totalPages"`
}

type AuditResetResult struct {
	ResetAt       string `json:"resetAt"`
	ResetBy       string `json:"resetBy"`
	DeletedEvents int    `json:"deletedEvents"`
}

type RedeemedCredential struct {
	Actor        string
	CredentialID string
	DBHost       string `json:"dbHost"`
	DBPort       int    `json:"dbPort"`
	DBUser       string `json:"dbUser"`
	DBPassword   string `json:"dbPassword"`
}

type BootstrapResult struct {
	Applied     bool
	Credentials int
	Mappings    int
}

func Open(ctx context.Context, options Options, cipher *pmacrypto.Cipher) (*Store, error) {
	driver := strings.ToLower(strings.TrimSpace(options.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	driverName, dsn, err := openTarget(driver, options)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, cipher: cipher, dialect: driver}, nil
}

func openTarget(driver string, options Options) (string, string, error) {
	switch driver {
	case "sqlite":
		if strings.TrimSpace(options.Path) == "" {
			return "", "", fmt.Errorf("%w: sqlite database path is required", ErrInvalidArgument)
		}
		return "sqlite", options.Path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", nil
	case "mysql":
		if strings.TrimSpace(options.DSN) == "" {
			return "", "", fmt.Errorf("%w: mysql DSN is required", ErrInvalidArgument)
		}
		return "mysql", options.DSN, nil
	default:
		return "", "", fmt.Errorf("%w: unsupported database driver %q", ErrInvalidArgument, driver)
	}
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	statements := s.migrationStatements()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.insertMigrationSQL(), 1, nowString()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) migrationStatements() []string {
	if s.dialect == "mysql" {
		return []string{
			`CREATE TABLE IF NOT EXISTS schema_migrations (
				version INT PRIMARY KEY,
				applied_at VARCHAR(64) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS credentials (
				id VARCHAR(191) PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				db_host VARCHAR(255) NOT NULL,
				db_port INT NOT NULL,
				db_user VARCHAR(255) NOT NULL,
				encrypted_db_password BLOB NOT NULL,
				password_nonce VARBINARY(64) NOT NULL,
				description TEXT,
				enabled BOOLEAN NOT NULL DEFAULT TRUE,
				created_at VARCHAR(64) NOT NULL,
				updated_at VARCHAR(64) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS mappings (
				id VARCHAR(191) PRIMARY KEY,
				subject_type VARCHAR(16) NOT NULL,
				subject VARCHAR(255) NOT NULL,
				credential_id VARCHAR(191) NOT NULL,
				created_at VARCHAR(64) NOT NULL,
				updated_at VARCHAR(64) NOT NULL,
				UNIQUE KEY uniq_mapping_subject_credential (subject_type, subject, credential_id),
				KEY idx_mappings_credential_id (credential_id),
				CONSTRAINT fk_mappings_credential FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS audit_events (
				id VARCHAR(191) PRIMARY KEY,
				timestamp VARCHAR(64) NOT NULL,
				actor VARCHAR(255) NOT NULL,
				actor_groups_json TEXT,
				action VARCHAR(191) NOT NULL,
				target_type VARCHAR(64) NOT NULL,
				target_id VARCHAR(191),
				result VARCHAR(32) NOT NULL,
				remote_address VARCHAR(255),
				user_agent TEXT,
				message TEXT,
				metadata_json TEXT,
				KEY idx_audit_events_timestamp (timestamp),
				KEY idx_audit_events_actor (actor),
				KEY idx_audit_events_action (action),
				KEY idx_audit_events_target_type (target_type),
				KEY idx_audit_events_result (result)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			`CREATE TABLE IF NOT EXISTS signon_tickets (
				id VARCHAR(191) PRIMARY KEY,
				ticket_hash VARCHAR(191) NOT NULL UNIQUE,
				actor VARCHAR(255) NOT NULL,
				credential_id VARCHAR(191) NOT NULL,
				expires_at VARCHAR(64) NOT NULL,
				used_at VARCHAR(64),
				created_at VARCHAR(64) NOT NULL,
				KEY idx_signon_tickets_hash (ticket_hash),
				KEY idx_signon_tickets_expires_at (expires_at),
				KEY idx_signon_tickets_actor (actor),
				KEY idx_signon_tickets_credential_id (credential_id),
				CONSTRAINT fk_signon_tickets_credential FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		}
	}
	return []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS credentials (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			db_host TEXT NOT NULL,
			db_port INTEGER NOT NULL,
			db_user TEXT NOT NULL,
			encrypted_db_password BLOB NOT NULL,
			password_nonce BLOB NOT NULL,
			description TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS mappings (
			id TEXT PRIMARY KEY,
			subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'group')),
			subject TEXT NOT NULL,
			credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(subject_type, subject, credential_id)
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			actor TEXT NOT NULL,
			actor_groups_json TEXT,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT,
			result TEXT NOT NULL,
			remote_address TEXT,
			user_agent TEXT,
			message TEXT,
			metadata_json TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(actor)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(action)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_target_type ON audit_events(target_type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_result ON audit_events(result)`,
		`CREATE TABLE IF NOT EXISTS signon_tickets (
			id TEXT PRIMARY KEY,
			ticket_hash TEXT NOT NULL UNIQUE,
			actor TEXT NOT NULL,
			credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
			expires_at TEXT NOT NULL,
			used_at TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_signon_tickets_hash ON signon_tickets(ticket_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_signon_tickets_expires_at ON signon_tickets(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_signon_tickets_actor ON signon_tickets(actor)`,
		`CREATE INDEX IF NOT EXISTS idx_signon_tickets_credential_id ON signon_tickets(credential_id)`,
	}
}

func (s *Store) insertMigrationSQL() string {
	if s.dialect == "mysql" {
		return `INSERT IGNORE INTO schema_migrations(version, applied_at) VALUES (?, ?)`
	}
	return `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (?, ?)`
}

func (s *Store) CreateCredential(ctx context.Context, input CredentialInput) (Credential, error) {
	if strings.TrimSpace(input.DBPassword) == "" {
		return Credential{}, fmt.Errorf("%w: dbPassword is required", ErrInvalidArgument)
	}
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		return Credential{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}
	if err := validateCredential(input, true); err != nil {
		return Credential{}, err
	}
	nonce, encrypted, err := s.cipher.Encrypt([]byte(input.DBPassword))
	if err != nil {
		return Credential{}, err
	}
	now := nowString()
	_, err = s.db.ExecContext(ctx, `INSERT INTO credentials
		(id, name, db_host, db_port, db_user, encrypted_db_password, password_nonce, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, input.DBHost, input.DBPort, input.DBUser, encrypted, nonce, input.Description, boolInt(input.Enabled), now, now)
	if err != nil {
		return Credential{}, err
	}
	return s.GetCredential(ctx, input.ID)
}

func (s *Store) UpdateCredential(ctx context.Context, id string, input CredentialInput) (Credential, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Credential{}, fmt.Errorf("%w: id is required", ErrInvalidArgument)
	}
	if input.ID == "" {
		input.ID = id
	}
	if input.ID != id {
		return Credential{}, fmt.Errorf("%w: body id must match path id", ErrInvalidArgument)
	}
	if err := validateCredential(input, false); err != nil {
		return Credential{}, err
	}
	now := nowString()
	var result sql.Result
	var err error
	if strings.TrimSpace(input.DBPassword) != "" {
		nonce, encrypted, err := s.cipher.Encrypt([]byte(input.DBPassword))
		if err != nil {
			return Credential{}, err
		}
		result, err = s.db.ExecContext(ctx, `UPDATE credentials SET
			name = ?, db_host = ?, db_port = ?, db_user = ?, encrypted_db_password = ?, password_nonce = ?,
			description = ?, enabled = ?, updated_at = ?
			WHERE id = ?`,
			input.Name, input.DBHost, input.DBPort, input.DBUser, encrypted, nonce, input.Description, boolInt(input.Enabled), now, id)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE credentials SET
			name = ?, db_host = ?, db_port = ?, db_user = ?, description = ?, enabled = ?, updated_at = ?
			WHERE id = ?`,
			input.Name, input.DBHost, input.DBPort, input.DBUser, input.Description, boolInt(input.Enabled), now, id)
	}
	if err != nil {
		return Credential{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return Credential{}, ErrNotFound
	}
	return s.GetCredential(ctx, id)
}

func (s *Store) GetCredential(ctx context.Context, id string) (Credential, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, db_host, db_port, db_user, description, enabled, created_at, updated_at
		FROM credentials WHERE id = ?`, id)
	return scanCredential(row)
}

func (s *Store) ListCredentials(ctx context.Context) ([]Credential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, db_host, db_port, db_user, description, enabled, created_at, updated_at
		FROM credentials ORDER BY LOWER(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCredentials(rows)
}

func (s *Store) DeleteCredential(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TestCredentialConnection(ctx context.Context, input CredentialTestInput) error {
	input.DBHost = strings.TrimSpace(input.DBHost)
	input.DBUser = strings.TrimSpace(input.DBUser)
	input.ExistingCredentialID = strings.TrimSpace(input.ExistingCredentialID)

	switch {
	case input.DBHost == "":
		return fmt.Errorf("%w: dbHost is required", ErrInvalidArgument)
	case input.DBPort <= 0 || input.DBPort > 65535:
		return fmt.Errorf("%w: dbPort must be between 1 and 65535", ErrInvalidArgument)
	case input.DBUser == "":
		return fmt.Errorf("%w: dbUser is required", ErrInvalidArgument)
	case input.Timeout <= 0:
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidArgument)
	}

	if strings.TrimSpace(input.DBPassword) == "" && input.ExistingCredentialID != "" {
		password, err := s.storedCredentialPassword(ctx, input.ExistingCredentialID)
		if err != nil {
			return err
		}
		input.DBPassword = password
	}
	if strings.TrimSpace(input.DBPassword) == "" {
		return fmt.Errorf("%w: dbPassword is required", ErrInvalidArgument)
	}

	cfg := mysqlDriver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(input.DBHost, strconv.Itoa(input.DBPort))
	cfg.User = input.DBUser
	cfg.Passwd = input.DBPassword
	cfg.Timeout = input.Timeout
	cfg.ReadTimeout = input.Timeout
	cfg.WriteTimeout = input.Timeout

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(input.Timeout)

	pingCtx, cancel := context.WithTimeout(ctx, input.Timeout)
	defer cancel()
	return db.PingContext(pingCtx)
}

func (s *Store) CreateMapping(ctx context.Context, input MappingInput) (Mapping, error) {
	if err := validateMapping(input); err != nil {
		return Mapping{}, err
	}
	if input.ID == "" {
		input.ID = mappingID(input.SubjectType, input.Subject, input.CredentialID)
	}
	now := nowString()
	_, err := s.db.ExecContext(ctx, `INSERT INTO mappings
		(id, subject_type, subject, credential_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		input.ID, input.SubjectType, input.Subject, input.CredentialID, now, now)
	if err != nil {
		return Mapping{}, err
	}
	return s.GetMapping(ctx, input.ID)
}

func (s *Store) GetMapping(ctx context.Context, id string) (Mapping, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, subject_type, subject, credential_id, created_at, updated_at
		FROM mappings WHERE id = ?`, id)
	return scanMapping(row)
}

func (s *Store) ListMappings(ctx context.Context) ([]Mapping, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, subject_type, subject, credential_id, created_at, updated_at
		FROM mappings ORDER BY subject_type, subject, credential_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mapping
	for rows.Next() {
		mapping, err := scanMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, mapping)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMapping(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM mappings WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AvailableCredentials(ctx context.Context, user string, groups []string) ([]Credential, error) {
	where, args := mappingSubjectWhere(user, groups)
	query := `SELECT DISTINCT c.id, c.name, c.db_host, c.db_port, c.db_user, c.description, c.enabled, c.created_at, c.updated_at
		FROM credentials c
		JOIN mappings m ON m.credential_id = c.id
		WHERE c.enabled = 1 AND (` + where + `)
		ORDER BY LOWER(c.name), c.id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCredentials(rows)
}

func (s *Store) UserCanUseCredential(ctx context.Context, user string, groups []string, credentialID string) (bool, error) {
	where, args := mappingSubjectWhere(user, groups)
	args = append([]any{credentialID}, args...)
	query := `SELECT COUNT(*)
		FROM credentials c
		JOIN mappings m ON m.credential_id = c.id
		WHERE c.id = ? AND c.enabled = 1 AND (` + where + `)`
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) CreateSignonTicket(ctx context.Context, actor, credentialID string, ttl time.Duration) (string, string, error) {
	if actor == "" || credentialID == "" {
		return "", "", fmt.Errorf("%w: actor and credentialId are required", ErrInvalidArgument)
	}
	rawTicket, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	id := "ticket_" + randomHex(12)
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO signon_tickets
		(id, ticket_hash, actor, credential_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, hashTicket(rawTicket), actor, credentialID, now.Add(ttl).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return "", "", err
	}
	return rawTicket, id, nil
}

func (s *Store) RedeemSignonTicket(ctx context.Context, rawTicket string) (RedeemedCredential, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RedeemedCredential{}, err
	}
	defer rollback(tx)

	var ticketID, actor, credentialID, expiresAt string
	var usedAt sql.NullString
	var encrypted, nonce []byte
	var dbHost, dbUser string
	var dbPort int
	err = tx.QueryRowContext(ctx, `SELECT t.id, t.actor, t.credential_id, t.expires_at, t.used_at,
			c.db_host, c.db_port, c.db_user, c.encrypted_db_password, c.password_nonce
		FROM signon_tickets t
		JOIN credentials c ON c.id = t.credential_id
		WHERE t.ticket_hash = ? AND c.enabled = 1`, hashTicket(rawTicket)).
		Scan(&ticketID, &actor, &credentialID, &expiresAt, &usedAt, &dbHost, &dbPort, &dbUser, &encrypted, &nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return RedeemedCredential{}, ErrTicketInvalid
	}
	if err != nil {
		return RedeemedCredential{}, err
	}
	if usedAt.Valid {
		return RedeemedCredential{}, ErrTicketUsed
	}
	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return RedeemedCredential{}, err
	}
	if !time.Now().UTC().Before(expiry) {
		return RedeemedCredential{}, ErrTicketExpired
	}
	password, err := s.cipher.Decrypt(nonce, encrypted)
	if err != nil {
		return RedeemedCredential{}, err
	}
	now := nowString()
	result, err := tx.ExecContext(ctx, `UPDATE signon_tickets SET used_at = ? WHERE id = ? AND used_at IS NULL`, now, ticketID)
	if err != nil {
		return RedeemedCredential{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return RedeemedCredential{}, ErrTicketUsed
	}
	if err := tx.Commit(); err != nil {
		return RedeemedCredential{}, err
	}
	return RedeemedCredential{
		Actor:        actor,
		CredentialID: credentialID,
		DBHost:       dbHost,
		DBPort:       dbPort,
		DBUser:       dbUser,
		DBPassword:   string(password),
	}, nil
}

func (s *Store) InsertAuditEvent(ctx context.Context, event AuditEvent) error {
	return insertAuditEvent(ctx, s.db, event)
}

func (s *Store) ListAuditEvents(ctx context.Context, filter AuditFilter) (AuditPage, error) {
	filter = normalizeAuditFilter(filter)
	where, args := auditWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`+where, args...).Scan(&total); err != nil {
		return AuditPage{}, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id, timestamp, actor, actor_groups_json, action, target_type, target_id,
			result, remote_address, user_agent, message, metadata_json
		FROM audit_events`+where+`
		ORDER BY timestamp DESC, id DESC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return AuditPage{}, err
	}
	defer rows.Close()
	var items []AuditEvent
	for rows.Next() {
		event, err := scanAuditEvent(rows)
		if err != nil {
			return AuditPage{}, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return AuditPage{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + filter.PageSize - 1) / filter.PageSize
	}
	return AuditPage{Items: items, Page: filter.Page, PageSize: filter.PageSize, TotalItems: total, TotalPages: totalPages}, nil
}

func (s *Store) ListRecentAuditActors(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT actor FROM audit_events
		ORDER BY timestamp DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{}, limit)
	actors := make([]string, 0, limit)
	for rows.Next() {
		var actor string
		if err := rows.Scan(&actor); err != nil {
			return nil, err
		}
		actor = strings.TrimSpace(actor)
		if actor == "" {
			continue
		}
		if _, ok := seen[actor]; ok {
			continue
		}
		seen[actor] = struct{}{}
		actors = append(actors, actor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return actors, nil
}

func (s *Store) ResetAuditEvents(ctx context.Context, actor string, actorGroups []string, remoteAddress, userAgent, reason string) (AuditResetResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuditResetResult{}, err
	}
	defer rollback(tx)

	var deleted int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&deleted); err != nil {
		return AuditResetResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM audit_events`); err != nil {
		return AuditResetResult{}, err
	}
	resetAt := nowString()
	metadata := map[string]any{"deletedEvents": deleted}
	if strings.TrimSpace(reason) != "" {
		metadata["reason"] = strings.TrimSpace(reason)
	}
	if err := insertAuditEvent(ctx, tx, AuditEvent{
		ID:            "audit_" + randomHex(16),
		Timestamp:     resetAt,
		Actor:         actor,
		ActorGroups:   actorGroups,
		Action:        auditmeta.ActionAuditReset,
		TargetType:    auditmeta.TargetTypeAudit,
		Result:        "success",
		RemoteAddress: remoteAddress,
		UserAgent:     userAgent,
		Message:       "Audit log was reset",
		Metadata:      metadata,
	}); err != nil {
		return AuditResetResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuditResetResult{}, err
	}
	return AuditResetResult{ResetAt: resetAt, ResetBy: actor, DeletedEvents: deleted}, nil
}

func (s *Store) ApplyBootstrap(ctx context.Context, boot config.Bootstrap) (BootstrapResult, error) {
	doc, err := config.ParseBootstrapDocument(boot.RawJSON)
	if err != nil {
		return BootstrapResult{}, err
	}
	if len(doc.Credentials) == 0 && len(doc.Mappings) == 0 {
		return BootstrapResult{}, nil
	}
	if err := validateBootstrap(doc); err != nil {
		return BootstrapResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BootstrapResult{}, err
	}
	defer rollback(tx)

	if boot.Mode == "first-run" {
		var existing int
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM credentials) + (SELECT COUNT(*) FROM mappings)`).Scan(&existing); err != nil {
			return BootstrapResult{}, err
		}
		if existing > 0 {
			return BootstrapResult{}, tx.Commit()
		}
	}

	for _, credential := range doc.Credentials {
		nonce, encrypted, err := s.cipher.Encrypt([]byte(credential.DBPassword))
		if err != nil {
			return BootstrapResult{}, err
		}
		now := nowString()
		_, err = tx.ExecContext(ctx, s.upsertCredentialSQL(),
			credential.ID, credential.Name, credential.DBHost, credential.DBPort, credential.DBUser, encrypted, nonce,
			credential.Description, boolInt(credential.Enabled), now, now)
		if err != nil {
			return BootstrapResult{}, err
		}
	}
	for _, mapping := range doc.Mappings {
		now := nowString()
		id := mappingID(mapping.SubjectType, mapping.Subject, mapping.CredentialID)
		_, err = tx.ExecContext(ctx, s.upsertMappingSQL(),
			id, mapping.SubjectType, mapping.Subject, mapping.CredentialID, now, now)
		if err != nil {
			return BootstrapResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return BootstrapResult{}, err
	}
	return BootstrapResult{Applied: true, Credentials: len(doc.Credentials), Mappings: len(doc.Mappings)}, nil
}

func (s *Store) upsertCredentialSQL() string {
	if s.dialect == "mysql" {
		return `INSERT INTO credentials
			(id, name, db_host, db_port, db_user, encrypted_db_password, password_nonce, description, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = VALUES(name),
				db_host = VALUES(db_host),
				db_port = VALUES(db_port),
				db_user = VALUES(db_user),
				encrypted_db_password = VALUES(encrypted_db_password),
				password_nonce = VALUES(password_nonce),
				description = VALUES(description),
				enabled = VALUES(enabled),
				updated_at = VALUES(updated_at)`
	}
	return `INSERT INTO credentials
		(id, name, db_host, db_port, db_user, encrypted_db_password, password_nonce, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			db_host = excluded.db_host,
			db_port = excluded.db_port,
			db_user = excluded.db_user,
			encrypted_db_password = excluded.encrypted_db_password,
			password_nonce = excluded.password_nonce,
			description = excluded.description,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`
}

func (s *Store) upsertMappingSQL() string {
	if s.dialect == "mysql" {
		return `INSERT INTO mappings
			(id, subject_type, subject, credential_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at)`
	}
	return `INSERT INTO mappings
		(id, subject_type, subject, credential_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(subject_type, subject, credential_id) DO UPDATE SET updated_at = excluded.updated_at`
}

func validateCredential(input CredentialInput, requirePassword bool) error {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.DBHost) == "" || strings.TrimSpace(input.DBUser) == "" {
		return fmt.Errorf("%w: id, name, dbHost, and dbUser are required", ErrInvalidArgument)
	}
	if input.DBPort <= 0 || input.DBPort > 65535 {
		return fmt.Errorf("%w: dbPort must be between 1 and 65535", ErrInvalidArgument)
	}
	if requirePassword && strings.TrimSpace(input.DBPassword) == "" {
		return fmt.Errorf("%w: dbPassword is required", ErrInvalidArgument)
	}
	return nil
}

func validateMapping(input MappingInput) error {
	if input.SubjectType != "user" && input.SubjectType != "group" {
		return fmt.Errorf("%w: subjectType must be user or group", ErrInvalidArgument)
	}
	if strings.TrimSpace(input.Subject) == "" || strings.TrimSpace(input.CredentialID) == "" {
		return fmt.Errorf("%w: subject and credentialId are required", ErrInvalidArgument)
	}
	return nil
}

func validateBootstrap(doc config.BootstrapDocument) error {
	credentialIDs := map[string]struct{}{}
	for _, credential := range doc.Credentials {
		input := CredentialInput{
			ID: credential.ID, Name: credential.Name, DBHost: credential.DBHost, DBPort: credential.DBPort,
			DBUser: credential.DBUser, DBPassword: credential.DBPassword, Enabled: credential.Enabled,
		}
		if err := validateCredential(input, true); err != nil {
			return err
		}
		credentialIDs[credential.ID] = struct{}{}
	}
	for _, mapping := range doc.Mappings {
		if err := validateMapping(MappingInput{
			SubjectType:  mapping.SubjectType,
			Subject:      mapping.Subject,
			CredentialID: mapping.CredentialID,
		}); err != nil {
			return err
		}
		if _, ok := credentialIDs[mapping.CredentialID]; !ok {
			return fmt.Errorf("%w: mapping references unknown bootstrap credential %q", ErrInvalidArgument, mapping.CredentialID)
		}
	}
	return nil
}

func scanCredential(scanner interface {
	Scan(dest ...any) error
}) (Credential, error) {
	var credential Credential
	var enabled int
	err := scanner.Scan(&credential.ID, &credential.Name, &credential.DBHost, &credential.DBPort, &credential.DBUser,
		&credential.Description, &enabled, &credential.CreatedAt, &credential.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	credential.Enabled = enabled != 0
	return credential, nil
}

func scanCredentials(rows *sql.Rows) ([]Credential, error) {
	var out []Credential
	for rows.Next() {
		credential, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, credential)
	}
	return out, rows.Err()
}

func (s *Store) storedCredentialPassword(ctx context.Context, id string) (string, error) {
	var encrypted, nonce []byte
	err := s.db.QueryRowContext(ctx, `SELECT encrypted_db_password, password_nonce FROM credentials WHERE id = ?`, id).Scan(&encrypted, &nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	password, err := s.cipher.Decrypt(nonce, encrypted)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func scanMapping(scanner interface {
	Scan(dest ...any) error
}) (Mapping, error) {
	var mapping Mapping
	err := scanner.Scan(&mapping.ID, &mapping.SubjectType, &mapping.Subject, &mapping.CredentialID, &mapping.CreatedAt, &mapping.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Mapping{}, ErrNotFound
	}
	return mapping, err
}

type auditScanner interface {
	Scan(dest ...any) error
}

func scanAuditEvent(scanner auditScanner) (AuditEvent, error) {
	var event AuditEvent
	var groupsJSON, metadataJSON sql.NullString
	err := scanner.Scan(&event.ID, &event.Timestamp, &event.Actor, &groupsJSON, &event.Action, &event.TargetType,
		&event.TargetID, &event.Result, &event.RemoteAddress, &event.UserAgent, &event.Message, &metadataJSON)
	if err != nil {
		return AuditEvent{}, err
	}
	if groupsJSON.Valid && groupsJSON.String != "" {
		_ = json.Unmarshal([]byte(groupsJSON.String), &event.ActorGroups)
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		_ = json.Unmarshal([]byte(metadataJSON.String), &event.Metadata)
	}
	return event, nil
}

type auditInserter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertAuditEvent(ctx context.Context, exec auditInserter, event AuditEvent) error {
	if event.ID == "" {
		event.ID = "audit_" + randomHex(16)
	}
	if event.Timestamp == "" {
		event.Timestamp = nowString()
	}
	if event.Actor == "" {
		event.Actor = "system"
	}
	groupsJSON, _ := json.Marshal(event.ActorGroups)
	metadataJSON, _ := json.Marshal(event.Metadata)
	_, err := exec.ExecContext(ctx, `INSERT INTO audit_events
		(id, timestamp, actor, actor_groups_json, action, target_type, target_id, result, remote_address, user_agent, message, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.Timestamp, event.Actor, string(groupsJSON), event.Action, event.TargetType, event.TargetID,
		event.Result, event.RemoteAddress, event.UserAgent, event.Message, string(metadataJSON))
	return err
}

func normalizeAuditFilter(filter AuditFilter) AuditFilter {
	if filter.Page < 1 {
		filter.Page = 1
	}
	switch filter.PageSize {
	case 10, 25, 50, 100:
	default:
		filter.PageSize = 25
	}
	return filter
}

func auditWhere(filter AuditFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(column, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, strings.TrimSpace(value))
	}
	add("actor", filter.Actor)
	add("action", filter.Action)
	add("target_type", filter.TargetType)
	add("result", filter.Result)
	if strings.TrimSpace(filter.From) != "" {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, strings.TrimSpace(filter.From))
	}
	if strings.TrimSpace(filter.To) != "" {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, strings.TrimSpace(filter.To))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func mappingSubjectWhere(user string, groups []string) (string, []any) {
	clauses := []string{"(m.subject_type = 'user' AND m.subject = ?)"}
	args := []any{user}
	if len(groups) > 0 {
		placeholders := make([]string, len(groups))
		for i, group := range groups {
			placeholders[i] = "?"
			args = append(args, group)
		}
		clauses = append(clauses, "(m.subject_type = 'group' AND m.subject IN ("+strings.Join(placeholders, ",")+"))")
	}
	return strings.Join(clauses, " OR "), args
}

func mappingID(subjectType, subject, credentialID string) string {
	sum := sha256.Sum256([]byte(subjectType + "\x00" + subject + "\x00" + credentialID))
	return "map_" + hex.EncodeToString(sum[:12])
}

func hashTicket(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}
