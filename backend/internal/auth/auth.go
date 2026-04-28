package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/michibiki-io/pma-gateway/backend/internal/config"
)

var (
	ErrUntrustedSource = errors.New("identity headers are not trusted from this source")
	ErrMissingUser     = errors.New("missing authenticated user header")
)

type Identity struct {
	User          string   `json:"user"`
	Groups        []string `json:"groups"`
	IsAdmin       bool     `json:"isAdmin"`
	RemoteAddress string   `json:"-"`
	UserAgent     string   `json:"-"`
}

func FromRequest(r *http.Request, cfg config.Config) (Identity, error) {
	if !cfg.IsTrustedRemote(r.RemoteAddr) {
		return Identity{}, ErrUntrustedSource
	}
	user := strings.TrimSpace(r.Header.Get(cfg.UserHeader))
	if user == "" {
		return Identity{}, ErrMissingUser
	}
	groups := splitGroups(r.Header.Get(cfg.GroupsHeader), cfg.GroupsSeparator)
	remoteAddress := r.RemoteAddr
	if cfg.TrustProxyHeaders {
		if forwarded := firstHeaderValue(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			remoteAddress = forwarded
		} else if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			remoteAddress = realIP
		}
	}
	return Identity{
		User:          user,
		Groups:        groups,
		IsAdmin:       cfg.IsAdmin(user, groups),
		RemoteAddress: remoteAddress,
		UserAgent:     r.UserAgent(),
	}, nil
}

func splitGroups(value, separator string) []string {
	if separator == "" {
		separator = ","
	}
	var groups []string
	for _, group := range strings.Split(value, separator) {
		group = strings.TrimSpace(group)
		if group != "" {
			groups = append(groups, group)
		}
	}
	return groups
}

func firstHeaderValue(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}
