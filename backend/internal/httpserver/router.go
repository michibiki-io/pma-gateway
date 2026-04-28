package httpserver

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/michibiki-io/pma-gateway/backend/internal/auditmeta"
	"github.com/michibiki-io/pma-gateway/backend/internal/auth"
	"github.com/michibiki-io/pma-gateway/backend/internal/buildinfo"
	"github.com/michibiki-io/pma-gateway/backend/internal/config"
	"github.com/michibiki-io/pma-gateway/backend/internal/storage"
	"go.uber.org/zap"
)

const auditActorSuggestionLimit = 200

type server struct {
	cfg    config.Config
	store  *storage.Store
	logger *zap.Logger
}

func NewRouter(cfg config.Config, store *storage.Store, logger *zap.Logger) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	s := &server{cfg: cfg, store: store, logger: logger}
	router := gin.New()
	router.Use(gin.Recovery(), securityHeaders())
	if len(cfg.AllowedOrigins) > 0 {
		router.Use(cors(cfg.AllowedOrigins))
	}

	router.GET("/healthz", s.healthz)
	router.GET("/readyz", s.readyz)
	if prefixed := config.JoinURLPath(cfg.Paths.PublicBasePath, "/healthz"); prefixed != "/healthz" {
		router.GET(prefixed, s.healthz)
	}
	if prefixed := config.JoinURLPath(cfg.Paths.PublicBasePath, "/readyz"); prefixed != "/readyz" {
		router.GET(prefixed, s.readyz)
	}

	router.GET(cfg.Paths.PublicEntryPath(), func(c *gin.Context) {
		c.Redirect(http.StatusFound, cfg.Paths.PMABasePath())
	})
	router.GET(config.JoinURLPath(cfg.Paths.FrontendBasePath(), "config.js"), s.frontendConfig)

	api := router.Group(cfg.Paths.APIBasePath())
	api.Use(s.csrf(), s.appCheck(), s.authRequired())
	api.GET("/me", s.me)
	api.GET("/available-credentials", s.availableCredentials)
	api.POST("/pma/sessions", s.createPMASession)

	admin := api.Group("/admin")
	admin.Use(s.adminRequired())
	admin.GET("/credentials", s.listCredentials)
	admin.POST("/credentials", s.createCredential)
	admin.POST("/credentials/test", s.testCredentialConnection)
	admin.GET("/credentials/:id", s.getCredential)
	admin.PUT("/credentials/:id", s.updateCredential)
	admin.DELETE("/credentials/:id", s.deleteCredential)
	admin.GET("/mappings", s.listMappings)
	admin.POST("/mappings", s.createMapping)
	admin.DELETE("/mappings/:id", s.deleteMapping)
	admin.GET("/audit-events/filter-options", s.listAuditFilterOptions)
	admin.GET("/audit-events", s.listAuditEvents)
	admin.POST("/audit-events/reset", s.resetAuditEvents)

	internal := router.Group("/internal/v1")
	internal.POST("/signon/redeem", s.redeemSignon)
	return router
}

func (s *server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": buildinfo.Current()})
}

func (s *server) readyz(c *gin.Context) {
	if len(s.cfg.MasterKey) != 32 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "master key not loaded", "version": buildinfo.Current()})
		return
	}
	if err := s.store.Ready(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "storage not ready", "version": buildinfo.Current()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "version": buildinfo.Current()})
}

func (s *server) frontendConfig(c *gin.Context) {
	version := buildinfo.Current()
	payload := map[string]any{
		"publicBasePath": emptyAsSlash(s.cfg.Paths.PublicBasePath),
		"frontendBase":   s.cfg.Paths.FrontendBasePath(),
		"apiBase":        s.cfg.Paths.APIBasePath(),
		"pmaBase":        s.cfg.Paths.PMABasePath(),
		"signonUrl":      s.cfg.Paths.SignonURL(),
		"version": map[string]any{
			"appVersion":        version.AppVersion,
			"appDisplayVersion": version.AppDisplayVersion,
			"appCommit":         version.AppCommit,
			"appShortCommit":    version.AppShortCommit,
			"phpMyAdminVersion": version.PHPMyAdminVersion,
		},
		"appCheck": map[string]any{
			"enabled":        s.cfg.AppCheckMode != "disabled",
			"headerName":     envFallback("VITE_APPCHECK_HEADER_NAME", "X-Firebase-AppCheck"),
			"exchangeUrl":    envFallback("VITE_APPCHECK_EXCHANGE_URL", "/appcheck/api/v1/exchange"),
			"turnstileSite":  envFallback("VITE_TURNSTILE_SITE_KEY", ""),
			"firebaseAPIKey": envFallback("VITE_FIREBASE_API_KEY", ""),
			"firebaseAppID":  envFallback("VITE_FIREBASE_APP_ID", ""),
			"firebaseProjID": envFallback("VITE_FIREBASE_PROJECT_ID", ""),
		},
	}
	encoded, _ := json.Marshal(payload)
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.String(http.StatusOK, "window.__PMA_GATEWAY_CONFIG__ = %s;\n", encoded)
}

func (s *server) me(c *gin.Context) {
	identity := mustIdentity(c)
	c.JSON(http.StatusOK, gin.H{"user": identity.User, "groups": identity.Groups, "isAdmin": identity.IsAdmin})
}

func (s *server) availableCredentials(c *gin.Context) {
	identity := mustIdentity(c)
	items, err := s.store.AvailableCredentials(c.Request.Context(), identity.User, identity.Groups)
	if err != nil {
		s.internalError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionCredentialAvailableList,
		TargetType:  auditmeta.TargetTypeCredential,
		Result:      "success",
		Message:     "User viewed available credentials",
		Metadata:    map[string]any{"count": len(items)},
	})
	c.JSON(http.StatusOK, gin.H{"items": s.formatCredentials(items)})
}

func (s *server) createPMASession(c *gin.Context) {
	identity := mustIdentity(c)
	var request struct {
		CredentialID string `json:"credentialId"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.CredentialID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credentialId is required"})
		return
	}
	allowed, err := s.store.UserCanUseCredential(c.Request.Context(), identity.User, identity.Groups, request.CredentialID)
	if err != nil {
		s.internalError(c, err)
		return
	}
	if !allowed {
		_ = s.audit(c, storage.AuditEvent{
			Actor:       identity.User,
			ActorGroups: identity.Groups,
			Action:      auditmeta.ActionSessionStart,
			TargetType:  auditmeta.TargetTypeCredential,
			TargetID:    request.CredentialID,
			Result:      "denied",
			Message:     "User attempted to start a phpMyAdmin session without a mapping",
		})
		c.JSON(http.StatusForbidden, gin.H{"error": "credential is not available to this user"})
		return
	}
	ticket, ticketID, err := s.store.CreateSignonTicket(c.Request.Context(), identity.User, request.CredentialID, s.cfg.TicketTTL)
	if err != nil {
		s.internalError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionTicketCreate,
		TargetType:  auditmeta.TargetTypeTicket,
		TargetID:    ticketID,
		Result:      "success",
		Message:     "Login ticket created",
		Metadata:    map[string]any{"credentialId": request.CredentialID, "ttlSeconds": int(s.cfg.TicketTTL.Seconds())},
	})
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionSessionStart,
		TargetType:  auditmeta.TargetTypeCredential,
		TargetID:    request.CredentialID,
		Result:      "success",
		Message:     "User started phpMyAdmin session",
	})
	redirect := s.cfg.Paths.SignonURL() + "?ticket=" + url.QueryEscape(ticket)
	c.JSON(http.StatusCreated, gin.H{"redirectUrl": redirect})
}

func (s *server) listCredentials(c *gin.Context) {
	items, err := s.store.ListCredentials(c.Request.Context())
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": s.formatCredentials(items)})
}

func (s *server) createCredential(c *gin.Context) {
	identity := mustIdentity(c)
	input, err := bindCredentialInput(c, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	credential, err := s.store.CreateCredential(c.Request.Context(), input)
	if err != nil {
		statusError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionCredentialCreate,
		TargetType:  auditmeta.TargetTypeCredential,
		TargetID:    credential.ID,
		Result:      "success",
		Message:     "Credential created",
	})
	c.JSON(http.StatusCreated, s.formatCredential(credential))
}

func (s *server) testCredentialConnection(c *gin.Context) {
	var request struct {
		ExistingCredentialID string `json:"existingCredentialId"`
		DBHost               string `json:"dbHost"`
		DBPort               int    `json:"dbPort"`
		DBUser               string `json:"dbUser"`
		DBPassword           string `json:"dbPassword"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	err := s.store.TestCredentialConnection(c.Request.Context(), storage.CredentialTestInput{
		ExistingCredentialID: request.ExistingCredentialID,
		DBHost:               request.DBHost,
		DBPort:               request.DBPort,
		DBUser:               request.DBUser,
		DBPassword:           request.DBPassword,
		Timeout:              s.cfg.CredentialTestTimeout,
	})
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	if errors.Is(err, storage.ErrInvalidArgument) || errors.Is(err, storage.ErrNotFound) {
		statusError(c, err)
		return
	}

	s.logger.Info("credential connection test failed",
		zap.String("dbHost", strings.TrimSpace(request.DBHost)),
		zap.Int("dbPort", request.DBPort),
		zap.String("dbUser", strings.TrimSpace(request.DBUser)),
		zap.Error(err),
	)
	c.JSON(http.StatusOK, gin.H{"success": false})
}

func (s *server) getCredential(c *gin.Context) {
	credential, err := s.store.GetCredential(c.Request.Context(), c.Param("id"))
	if err != nil {
		statusError(c, err)
		return
	}
	c.JSON(http.StatusOK, s.formatCredential(credential))
}

func (s *server) updateCredential(c *gin.Context) {
	identity := mustIdentity(c)
	input, err := bindCredentialInput(c, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	credential, err := s.store.UpdateCredential(c.Request.Context(), c.Param("id"), input)
	if err != nil {
		statusError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionCredentialUpdate,
		TargetType:  auditmeta.TargetTypeCredential,
		TargetID:    credential.ID,
		Result:      "success",
		Message:     "Credential updated",
	})
	c.JSON(http.StatusOK, s.formatCredential(credential))
}

func (s *server) deleteCredential(c *gin.Context) {
	identity := mustIdentity(c)
	id := c.Param("id")
	if err := s.store.DeleteCredential(c.Request.Context(), id); err != nil {
		statusError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionCredentialDelete,
		TargetType:  auditmeta.TargetTypeCredential,
		TargetID:    id,
		Result:      "success",
		Message:     "Credential deleted",
	})
	c.Status(http.StatusNoContent)
}

func (s *server) listMappings(c *gin.Context) {
	items, err := s.store.ListMappings(c.Request.Context())
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": s.formatMappings(items)})
}

func (s *server) createMapping(c *gin.Context) {
	identity := mustIdentity(c)
	var input storage.MappingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	mapping, err := s.store.CreateMapping(c.Request.Context(), input)
	if err != nil {
		statusError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionMappingCreate,
		TargetType:  auditmeta.TargetTypeMapping,
		TargetID:    mapping.ID,
		Result:      "success",
		Message:     "Mapping created",
		Metadata:    map[string]any{"credentialId": mapping.CredentialID, "subjectType": mapping.SubjectType, "subject": mapping.Subject},
	})
	c.JSON(http.StatusCreated, s.formatMapping(mapping))
}

func (s *server) deleteMapping(c *gin.Context) {
	identity := mustIdentity(c)
	id := c.Param("id")
	if err := s.store.DeleteMapping(c.Request.Context(), id); err != nil {
		statusError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionMappingDelete,
		TargetType:  auditmeta.TargetTypeMapping,
		TargetID:    id,
		Result:      "success",
		Message:     "Mapping deleted",
	})
	c.Status(http.StatusNoContent)
}

func (s *server) listAuditFilterOptions(c *gin.Context) {
	actors, err := s.store.ListRecentAuditActors(c.Request.Context(), auditActorSuggestionLimit)
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"actions":          auditmeta.ActionValues(),
		"targetTypes":      auditmeta.TargetTypeValues(),
		"actorSuggestions": actors,
	})
}

func (s *server) listAuditEvents(c *gin.Context) {
	identity := mustIdentity(c)
	filter := storage.AuditFilter{
		Actor:      c.Query("actor"),
		Action:     c.Query("action"),
		TargetType: c.Query("targetType"),
		Result:     c.Query("result"),
		From:       c.Query("from"),
		To:         c.Query("to"),
		Page:       queryInt(c, "page", 1),
		PageSize:   queryInt(c, "pageSize", 25),
	}
	page, err := s.store.ListAuditEvents(c.Request.Context(), filter)
	if err != nil {
		s.internalError(c, err)
		return
	}
	_ = s.audit(c, storage.AuditEvent{
		Actor:       identity.User,
		ActorGroups: identity.Groups,
		Action:      auditmeta.ActionAuditView,
		TargetType:  auditmeta.TargetTypeAudit,
		Result:      "success",
		Message:     "Admin viewed audit logs",
	})
	c.JSON(http.StatusOK, s.formatAuditPage(page))
}

func (s *server) resetAuditEvents(c *gin.Context) {
	identity := mustIdentity(c)
	var request struct {
		Confirmation string `json:"confirmation"`
		Reason       string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if request.Confirmation != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation must be RESET"})
		return
	}
	result, err := s.store.ResetAuditEvents(c.Request.Context(), identity.User, identity.Groups, identity.RemoteAddress, identity.UserAgent, request.Reason)
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, s.formatAuditResetResult(result))
}

func (s *server) redeemSignon(c *gin.Context) {
	if !s.cfg.IsTrustedRemote(c.Request.RemoteAddr) {
		c.JSON(http.StatusForbidden, gin.H{"error": "internal endpoint is only available to trusted local callers"})
		return
	}
	if !hmac.Equal([]byte(c.GetHeader("X-PMA-Gateway-Internal-Secret")), []byte(s.cfg.InternalSharedSecret)) {
		_ = s.store.InsertAuditEvent(c.Request.Context(), storage.AuditEvent{
			Actor:         "system",
			Action:        auditmeta.ActionTicketRedeem,
			TargetType:    auditmeta.TargetTypeTicket,
			Result:        "denied",
			RemoteAddress: c.ClientIP(),
			Message:       "Login ticket redemption denied",
		})
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid internal secret"})
		return
	}
	var request struct {
		Ticket string `json:"ticket"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Ticket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ticket is required"})
		return
	}
	redeemed, err := s.store.RedeemSignonTicket(c.Request.Context(), request.Ticket)
	if err != nil {
		_ = s.store.InsertAuditEvent(c.Request.Context(), storage.AuditEvent{
			Actor:         "system",
			Action:        auditmeta.ActionTicketRedeem,
			TargetType:    auditmeta.TargetTypeTicket,
			Result:        "failure",
			RemoteAddress: c.ClientIP(),
			Message:       "Login ticket redemption failed",
			Metadata:      map[string]any{"error": publicTicketError(err)},
		})
		status := http.StatusUnauthorized
		if errors.Is(err, storage.ErrTicketExpired) || errors.Is(err, storage.ErrTicketUsed) {
			status = http.StatusGone
		}
		c.JSON(status, gin.H{"error": publicTicketError(err)})
		return
	}
	_ = s.store.InsertAuditEvent(c.Request.Context(), storage.AuditEvent{
		Actor:      redeemed.Actor,
		Action:     auditmeta.ActionTicketRedeem,
		TargetType: auditmeta.TargetTypeCredential,
		TargetID:   redeemed.CredentialID,
		Result:     "success",
		Message:    "Login ticket redeemed",
	})
	c.JSON(http.StatusOK, gin.H{
		"dbHost":     redeemed.DBHost,
		"dbPort":     redeemed.DBPort,
		"dbUser":     redeemed.DBUser,
		"dbPassword": redeemed.DBPassword,
	})
}

func (s *server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := auth.FromRequest(c.Request, s.cfg)
		if err != nil {
			_ = s.store.InsertAuditEvent(c.Request.Context(), storage.AuditEvent{
				Actor:         "anonymous",
				Action:        auditmeta.ActionAPIUnauthorized,
				TargetType:    auditmeta.TargetTypeSystem,
				Result:        "denied",
				RemoteAddress: c.ClientIP(),
				UserAgent:     c.Request.UserAgent(),
				Message:       "Unauthorized API access",
				Metadata:      map[string]any{"reason": err.Error()},
			})
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authenticated identity is required"})
			return
		}
		c.Set("identity", identity)
		c.Next()
	}
}

func (s *server) adminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := mustIdentity(c)
		if !identity.IsAdmin {
			_ = s.audit(c, storage.AuditEvent{
				Actor:       identity.User,
				ActorGroups: identity.Groups,
				Action:      auditmeta.ActionAPIAdminUnauthorized,
				TargetType:  auditmeta.TargetTypeSystem,
				Result:      "denied",
				Message:     "Unauthorized admin API access",
			})
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin authorization is required"})
			return
		}
		c.Next()
	}
}

func (s *server) csrf() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		referer := c.GetHeader("Referer")
		if origin == "" && referer == "" {
			c.Next()
			return
		}
		if origin != "" && s.originAllowed(c, origin) {
			c.Next()
			return
		}
		if referer != "" && s.originAllowed(c, referer) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "same-origin request required"})
	}
}

func (s *server) appCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch s.cfg.AppCheckMode {
		case "disabled":
			c.Next()
		case "trusted-header", "required":
			value := strings.ToLower(strings.TrimSpace(c.GetHeader(s.cfg.AppCheckVerifiedHeader)))
			if value == "true" || value == "1" || value == "yes" {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "verified app check header is required"})
		default:
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "app check mode is invalid"})
		}
	}
}

func (s *server) originAllowed(c *gin.Context, raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for _, allowed := range s.cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(raw, "/"), strings.TrimRight(allowed, "/")) {
			return true
		}
	}
	host := c.Request.Host
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" && s.cfg.TrustProxyHeaders && s.cfg.IsTrustedRemote(c.Request.RemoteAddr) {
		host = forwardedHost
	}
	return strings.EqualFold(parsed.Host, host)
}

func (s *server) audit(c *gin.Context, event storage.AuditEvent) error {
	identity, _ := c.Get("identity")
	if event.RemoteAddress == "" {
		if value, ok := identity.(auth.Identity); ok {
			event.RemoteAddress = value.RemoteAddress
			event.UserAgent = value.UserAgent
		}
	}
	return s.store.InsertAuditEvent(c.Request.Context(), event)
}

func (s *server) internalError(c *gin.Context, err error) {
	s.logger.Error("request failed", zap.Error(err), zap.String("path", c.Request.URL.Path), zap.String("method", c.Request.Method))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

type credentialRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DBHost      string `json:"dbHost"`
	DBPort      int    `json:"dbPort"`
	DBUser      string `json:"dbUser"`
	DBPassword  string `json:"dbPassword"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

func bindCredentialInput(c *gin.Context, create bool) (storage.CredentialInput, error) {
	var request credentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return storage.CredentialInput{}, errors.New("invalid JSON body")
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	if !create && request.Enabled == nil {
		enabled = true
	}
	return storage.CredentialInput{
		ID:          request.ID,
		Name:        request.Name,
		DBHost:      request.DBHost,
		DBPort:      request.DBPort,
		DBUser:      request.DBUser,
		DBPassword:  request.DBPassword,
		Description: request.Description,
		Enabled:     enabled,
	}, nil
}

func statusError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	case errors.Is(err, storage.ErrInvalidArgument):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func publicTicketError(err error) string {
	switch {
	case errors.Is(err, storage.ErrTicketExpired):
		return "ticket expired"
	case errors.Is(err, storage.ErrTicketUsed):
		return "ticket already used"
	default:
		return "ticket invalid"
	}
}

func mustIdentity(c *gin.Context) auth.Identity {
	value, exists := c.Get("identity")
	if !exists {
		panic("identity missing from context")
	}
	identity, ok := value.(auth.Identity)
	if !ok {
		panic("identity has unexpected type")
	}
	return identity
}

func queryInt(c *gin.Context, name string, fallback int) int {
	value := c.Query(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Next()
	}
}

func cors(allowedOrigins []string) gin.HandlerFunc {
	allowed := map[string]struct{}{}
	for _, origin := range allowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", c.GetHeader("Origin"))
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Firebase-AppCheck, X-AppCheck-Verified")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func emptyAsSlash(value string) string {
	if value == "" {
		return "/"
	}
	return value
}

func envFallback(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
