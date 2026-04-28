package auditmeta

const (
	ActionBootstrapApply          = "bootstrap.apply"
	ActionCredentialAvailableList = "credential.available.list"
	ActionSessionStart            = "session.start"
	ActionTicketCreate            = "ticket.create"
	ActionTicketRedeem            = "ticket.redeem"
	ActionCredentialCreate        = "credential.create"
	ActionCredentialUpdate        = "credential.update"
	ActionCredentialDelete        = "credential.delete"
	ActionMappingCreate           = "mapping.create"
	ActionMappingDelete           = "mapping.delete"
	ActionAuditView               = "audit.view"
	ActionAuditReset              = "audit.reset"
	ActionAPIUnauthorized         = "api.unauthorized"
	ActionAPIAdminUnauthorized    = "api.admin.unauthorized"
)

const (
	TargetTypeSystem     = "system"
	TargetTypeCredential = "credential"
	TargetTypeTicket     = "ticket"
	TargetTypeMapping    = "mapping"
	TargetTypeAudit      = "audit"
)

var actionValues = []string{
	ActionBootstrapApply,
	ActionCredentialAvailableList,
	ActionSessionStart,
	ActionTicketCreate,
	ActionTicketRedeem,
	ActionCredentialCreate,
	ActionCredentialUpdate,
	ActionCredentialDelete,
	ActionMappingCreate,
	ActionMappingDelete,
	ActionAuditView,
	ActionAuditReset,
	ActionAPIUnauthorized,
	ActionAPIAdminUnauthorized,
}

var targetTypeValues = []string{
	TargetTypeSystem,
	TargetTypeCredential,
	TargetTypeTicket,
	TargetTypeMapping,
	TargetTypeAudit,
}

func ActionValues() []string {
	return append([]string(nil), actionValues...)
}

func TargetTypeValues() []string {
	return append([]string(nil), targetTypeValues...)
}
