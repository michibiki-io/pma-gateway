package httpserver

import "github.com/michibiki-io/pma-gateway/backend/internal/storage"

func (s *server) formatCredential(item storage.Credential) storage.Credential {
	item.CreatedAt = s.cfg.FormatTimestamp(item.CreatedAt)
	item.UpdatedAt = s.cfg.FormatTimestamp(item.UpdatedAt)
	return item
}

func (s *server) formatCredentials(items []storage.Credential) []storage.Credential {
	for index := range items {
		items[index] = s.formatCredential(items[index])
	}
	return items
}

func (s *server) formatMapping(item storage.Mapping) storage.Mapping {
	item.CreatedAt = s.cfg.FormatTimestamp(item.CreatedAt)
	item.UpdatedAt = s.cfg.FormatTimestamp(item.UpdatedAt)
	return item
}

func (s *server) formatMappings(items []storage.Mapping) []storage.Mapping {
	for index := range items {
		items[index] = s.formatMapping(items[index])
	}
	return items
}

func (s *server) formatAuditPage(page storage.AuditPage) storage.AuditPage {
	for index := range page.Items {
		page.Items[index].Timestamp = s.cfg.FormatTimestamp(page.Items[index].Timestamp)
	}
	return page
}

func (s *server) formatAuditResetResult(result storage.AuditResetResult) storage.AuditResetResult {
	result.ResetAt = s.cfg.FormatTimestamp(result.ResetAt)
	return result
}
