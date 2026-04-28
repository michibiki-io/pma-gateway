import {
  _,
  addMessages,
  getLocaleFromNavigator,
  init,
  locale,
  unwrapFunctionStore
} from 'svelte-i18n';
import en from './messages/en.js';
import ja from './messages/ja.js';

const DEFAULT_LOCALE = 'en';
const LOCALE_STORAGE_KEY = 'pma-gateway-locale';
const INVALID_ARGUMENT_PREFIX = 'invalid argument: ';
const UNKNOWN_BOOTSTRAP_CREDENTIAL_PATTERN =
  /^invalid argument: mapping references unknown bootstrap credential "([^"]+)"$/;
const HTML_LIKE_PATTERN = /^\s*<(?:!doctype|html|head|body|title|h1|p|div|span|pre|br)\b/i;

const formatMessage = unwrapFunctionStore(_);

export const supportedLocales = Object.freeze(['en', 'ja']);

const apiErrorKeyByMessage = Object.freeze({
  'Failed to fetch': 'errors.networkRequestFailed',
  'Load failed': 'errors.networkRequestFailed',
  'NetworkError when attempting to fetch resource.': 'errors.networkRequestFailed',
  'master key not loaded': 'errors.masterKeyNotLoaded',
  'storage not ready': 'errors.storageNotReady',
  'credentialId is required': 'errors.credentialIdRequired',
  'credential is not available to this user': 'errors.credentialUnavailable',
  'invalid JSON body': 'errors.invalidJsonBody',
  'confirmation must be RESET': 'errors.confirmationMustBeReset',
  'internal endpoint is only available to trusted local callers': 'errors.internalOnly',
  'invalid internal secret': 'errors.internalSecretInvalid',
  'ticket is required': 'errors.ticketRequired',
  'authenticated identity is required': 'errors.authenticatedIdentityRequired',
  'admin authorization is required': 'errors.adminAuthorizationRequired',
  'same-origin request required': 'errors.sameOriginRequired',
  'verified app check header is required': 'errors.verifiedAppCheckHeaderRequired',
  'app check mode is invalid': 'errors.appCheckModeInvalid',
  'internal server error': 'errors.internalServerError',
  'not found': 'errors.notFound',
  'ticket expired': 'errors.ticketExpired',
  'ticket already used': 'errors.ticketAlreadyUsed',
  'ticket invalid': 'errors.ticketInvalid'
});

const invalidArgumentKeyByMessage = Object.freeze({
  'dbHost is required': 'errors.dbHostRequired',
  'dbUser is required': 'errors.dbUserRequired',
  'dbPassword is required': 'errors.dbPasswordRequired',
  'id is required': 'errors.idRequired',
  'body id must match path id': 'errors.bodyIdMustMatch',
  'actor and credentialId are required': 'errors.actorCredentialRequired',
  'id, name, dbHost, and dbUser are required': 'errors.requiredCredentialFields',
  'dbPort must be between 1 and 65535': 'errors.dbPortRange',
  'subjectType must be user or group': 'errors.subjectTypeInvalid',
  'subject and credentialId are required': 'errors.subjectCredentialRequired'
});

const auditMessageKeyByMessage = Object.freeze({
  'Bootstrap import failed': 'audit.messages.bootstrapImportFailed',
  'Bootstrap import applied': 'audit.messages.bootstrapImportApplied',
  'User viewed available credentials': 'audit.messages.viewedAvailableCredentials',
  'User attempted to start a phpMyAdmin session without a mapping': 'audit.messages.sessionStartDenied',
  'Login ticket created': 'audit.messages.loginTicketCreated',
  'User started phpMyAdmin session': 'audit.messages.sessionStarted',
  'Credential created': 'audit.messages.credentialCreated',
  'Credential updated': 'audit.messages.credentialUpdated',
  'Credential deleted': 'audit.messages.credentialDeleted',
  'Mapping created': 'audit.messages.mappingCreated',
  'Mapping deleted': 'audit.messages.mappingDeleted',
  'Admin viewed audit logs': 'audit.messages.auditViewed',
  'Login ticket redemption denied': 'audit.messages.ticketRedemptionDenied',
  'Login ticket redemption failed': 'audit.messages.ticketRedemptionFailed',
  'Login ticket redeemed': 'audit.messages.ticketRedeemed',
  'Unauthorized API access': 'audit.messages.apiUnauthorized',
  'Unauthorized admin API access': 'audit.messages.adminUnauthorized',
  'Audit log was reset': 'audit.messages.auditReset'
});

const auditActionKeyByValue = Object.freeze({
  'bootstrap.apply': 'audit.actionLabels.bootstrapApply',
  'credential.available.list': 'audit.actionLabels.credentialAvailableList',
  'session.start': 'audit.actionLabels.sessionStart',
  'ticket.create': 'audit.actionLabels.ticketCreate',
  'ticket.redeem': 'audit.actionLabels.ticketRedeem',
  'credential.create': 'audit.actionLabels.credentialCreate',
  'credential.update': 'audit.actionLabels.credentialUpdate',
  'credential.delete': 'audit.actionLabels.credentialDelete',
  'mapping.create': 'audit.actionLabels.mappingCreate',
  'mapping.delete': 'audit.actionLabels.mappingDelete',
  'audit.view': 'audit.actionLabels.auditView',
  'audit.reset': 'audit.actionLabels.auditReset',
  'api.unauthorized': 'audit.actionLabels.apiUnauthorized',
  'api.admin.unauthorized': 'audit.actionLabels.adminUnauthorized'
});

const auditTargetTypeKeyByValue = Object.freeze({
  system: 'audit.targetTypeLabels.system',
  credential: 'audit.targetTypeLabels.credential',
  ticket: 'audit.targetTypeLabels.ticket',
  mapping: 'audit.targetTypeLabels.mapping',
  audit: 'audit.targetTypeLabels.audit'
});

let initialized = false;

addMessages('en', en);
addMessages('ja', ja);

function normalizeLocale(value) {
  if (!value) {
    return DEFAULT_LOCALE;
  }
  const normalized = value.toLowerCase().split('-')[0];
  return supportedLocales.includes(normalized) ? normalized : DEFAULT_LOCALE;
}

function readStoredLocale() {
  if (typeof window === 'undefined') {
    return '';
  }
  const storedLocale = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  return storedLocale ? normalizeLocale(storedLocale) : '';
}

export function setupI18n() {
  if (initialized) {
    return;
  }
  initialized = true;

  const initialLocale =
    typeof window === 'undefined'
      ? DEFAULT_LOCALE
      : readStoredLocale() || normalizeLocale(getLocaleFromNavigator());

  init({
    fallbackLocale: DEFAULT_LOCALE,
    initialLocale
  });

  if (typeof window !== 'undefined') {
    locale.subscribe((value) => {
      if (!value) {
        return;
      }
      window.localStorage.setItem(LOCALE_STORAGE_KEY, normalizeLocale(value));
    });
  }
}

export function setAppLocale(value) {
  locale.set(normalizeLocale(value));
}

export function translateApiError(message, status) {
  const normalized = typeof message === 'string' ? message.trim() : '';
  if (!normalized) {
    if (status === 500) {
      return formatMessage('errors.internalServerError');
    }
    if (status === 502 || status === 503 || status === 504) {
      return formatMessage('errors.serviceUnavailable');
    }
    return formatMessage('errors.requestFailed', { values: { status: status ?? 0 } });
  }

  if (HTML_LIKE_PATTERN.test(normalized)) {
    if (status === 500) {
      return formatMessage('errors.internalServerError');
    }
    if (status === 502 || status === 503 || status === 504) {
      return formatMessage('errors.serviceUnavailable');
    }
    return formatMessage('errors.requestFailed', { values: { status: status ?? 0 } });
  }

  const directKey = apiErrorKeyByMessage[normalized];
  if (directKey) {
    return formatMessage(directKey);
  }

  if (normalized === 'invalid argument') {
    return formatMessage('errors.invalidArgument');
  }

  if (normalized.startsWith(INVALID_ARGUMENT_PREFIX)) {
    const invalidArgumentMessage = normalized.slice(INVALID_ARGUMENT_PREFIX.length);
    const invalidArgumentKey = invalidArgumentKeyByMessage[invalidArgumentMessage];
    if (invalidArgumentKey) {
      return formatMessage(invalidArgumentKey);
    }

    const credentialMatch = normalized.match(UNKNOWN_BOOTSTRAP_CREDENTIAL_PATTERN);
    if (credentialMatch) {
      return formatMessage('errors.mappingUnknownBootstrapCredential', {
        values: { credentialId: credentialMatch[1] }
      });
    }
  }

  return normalized;
}

export function translateAuditMessage(message) {
  const normalized = typeof message === 'string' ? message.trim() : '';
  if (!normalized) {
    return '';
  }

  const key = auditMessageKeyByMessage[normalized];
  return key ? formatMessage(key) : normalized;
}

export function translateAuditAction(action) {
  const normalized = typeof action === 'string' ? action.trim() : '';
  if (!normalized) {
    return '';
  }
  const key = auditActionKeyByValue[normalized];
  return key ? formatMessage(key) : normalized;
}

export function translateAuditTargetType(targetType) {
  const normalized = typeof targetType === 'string' ? targetType.trim() : '';
  if (!normalized) {
    return '';
  }
  const key = auditTargetTypeKeyByValue[normalized];
  return key ? formatMessage(key) : normalized;
}

export { _, locale };
