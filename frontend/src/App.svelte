<script>
  import { onMount } from "svelte";
  import panelLeftContractIcon from "@fluentui/svg-icons/icons/panel_left_contract_20_regular.svg?raw";
  import panelLeftExpandIcon from "@fluentui/svg-icons/icons/panel_left_expand_20_regular.svg?raw";
  import { apiRequest, runtimeConfig } from "./lib/api.js";
  import AuditLogList from "./lib/AuditLogList.svelte";
  import CredentialTable from "./lib/CredentialTable.svelte";
  import KeyValueList from "./lib/KeyValueList.svelte";
  import MappingList from "./lib/MappingList.svelte";
  import {
    _,
    locale,
    setAppLocale,
    supportedLocales,
    translateApiError,
    translateAuditAction,
    translateAuditTargetType,
  } from "./lib/i18n/index.js";
  import {
    setThemeMode,
    supportedThemeModes,
    themePreference,
  } from "./lib/theme.js";

  const ROUTES = {
    DASHBOARD: "dashboard",
    ACCOUNT: "account",
    ADMIN_CREDENTIALS: "admin-credentials",
    ADMIN_MAPPINGS: "admin-mappings",
    ADMIN_AUDIT: "admin-audit",
  };
  const SUBJECT_TYPE_OPTIONS = ["user", "group"];
  const AUDIT_RESULT_OPTIONS = ["", "success", "failure", "denied"];
  const AUDIT_PAGE_SIZES = [10, 25, 50, 100];
  const AUDIT_RESET_CONFIRMATION = "RESET";
  const AUDIT_ACTOR_SUGGESTIONS_ID = "audit-actor-suggestions";
  const BRAND_ICON_CLASS = "fa-solid fa-database";
  const SIDEBAR_COLLAPSED_STORAGE_KEY = "pma-gateway.sidebar-collapsed";
  const NAV_ITEM_ICONS = {
    [ROUTES.DASHBOARD]: "fa-solid fa-key",
    [ROUTES.ACCOUNT]: "fa-solid fa-user",
    [ROUTES.ADMIN_CREDENTIALS]: "fa-solid fa-database",
    [ROUTES.ADMIN_MAPPINGS]: "fa-solid fa-diagram-project",
    [ROUTES.ADMIN_AUDIT]: "fa-solid fa-file-lines",
  };

  const config = runtimeConfig();
  const base = config.frontendBase.replace(/\/$/, "");

  let route = currentRoute();
  let mobileMenuOpen = false;
  let sidebarCollapsed = false;
  let localeMenuOpen = false;
  let themeMenuOpen = false;
  let me = null;
  let credentials = [];
  let mappings = [];
  let auditPage = {
    items: [],
    page: 1,
    pageSize: 25,
    totalItems: 0,
    totalPages: 0,
  };
  let auditMetadata = blankAuditMetadata();
  let auditFilters = blankAuditFilters();
  let loading = true;
  let saving = false;
  let error = null;
  let toast = "";
  let credentialForm = blankCredential();
  let credentialFieldErrors = blankCredentialFieldErrors();
  let credentialTest = blankCredentialTest();
  let mappingForm = blankMapping();
  let mappingFieldErrors = blankMappingFieldErrors();
  let editingCredentialId = "";
  let resetOpen = false;
  let aboutOpen = false;
  let resetConfirmation = "";
  let resetReason = "";
  let resetBusy = false;
  let deleteDialog = null;
  let deleteBusy = false;

  onMount(() => {
    sidebarCollapsed = readSidebarCollapsed();
    const handlePopState = () => {
      route = currentRoute();
      loadRoute();
    };
    const handleDocumentClick = (event) => {
      if (
        !(event.target instanceof Element) ||
        !event.target.closest(".locale-menu")
      ) {
        localeMenuOpen = false;
      }
      if (
        !(event.target instanceof Element) ||
        !event.target.closest(".theme-menu")
      ) {
        themeMenuOpen = false;
      }
    };
    const handleDocumentKeydown = (event) => {
      if (event.key === "Escape") {
        localeMenuOpen = false;
        themeMenuOpen = false;
        aboutOpen = false;
      }
    };
    window.addEventListener("popstate", handlePopState);
    document.addEventListener("click", handleDocumentClick);
    document.addEventListener("keydown", handleDocumentKeydown);
    loadInitial();
    return () => {
      window.removeEventListener("popstate", handlePopState);
      document.removeEventListener("click", handleDocumentClick);
      document.removeEventListener("keydown", handleDocumentKeydown);
    };
  });

  async function loadInitial() {
    loading = true;
    try {
      me = await apiRequest(config, "/me");
      await loadRoute();
    } catch (err) {
      setTranslatedError(err);
    } finally {
      loading = false;
    }
  }

  async function loadRoute() {
    error = null;
    if (!me) {
      return;
    }
    try {
      if (route === ROUTES.DASHBOARD) {
        await loadAvailableCredentials();
      } else if (route === ROUTES.ADMIN_CREDENTIALS && me.isAdmin) {
        await loadAdminCredentials();
      } else if (route === ROUTES.ADMIN_MAPPINGS && me.isAdmin) {
        await Promise.all([loadAdminCredentials(), loadMappings()]);
      } else if (route === ROUTES.ADMIN_AUDIT && me.isAdmin) {
        await Promise.all([loadAuditMetadata(), loadAudit(1)]);
      }
    } catch (err) {
      setTranslatedError(err);
    }
  }

  async function loadAvailableCredentials() {
    const body = await apiRequest(config, "/available-credentials");
    credentials = body.items || [];
  }

  async function loadAdminCredentials() {
    const body = await apiRequest(config, "/admin/credentials");
    credentials = body.items || [];
  }

  async function loadMappings() {
    const body = await apiRequest(config, "/admin/mappings");
    mappings = body.items || [];
  }

  async function loadAuditMetadata() {
    const body = await apiRequest(config, "/admin/audit-events/filter-options");
    auditMetadata = {
      actions: body.actions || [],
      targetTypes: body.targetTypes || [],
      actorSuggestions: body.actorSuggestions || [],
    };
  }

  async function loadAudit(page = auditPage.page || 1) {
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("pageSize", String(auditFilters.pageSize));
    for (const key of [
      "actor",
      "action",
      "targetType",
      "result",
      "from",
      "to",
    ]) {
      if (auditFilters[key]) {
        params.set(key, auditFilters[key]);
      }
    }
    auditPage = await apiRequest(
      config,
      `/admin/audit-events?${params.toString()}`,
    );
  }

  async function startSession(id) {
    saving = true;
    error = null;
    try {
      const body = await apiRequest(config, "/pma/sessions", {
        method: "POST",
        body: JSON.stringify({ credentialId: id }),
      });
      window.location.assign(body.redirectUrl);
    } catch (err) {
      setTranslatedError(err);
    } finally {
      saving = false;
    }
  }

  async function saveCredential() {
    saving = true;
    error = null;
    clearCredentialFieldErrors();
    const validationErrors = validateCredentialForm();
    if (hasFieldErrors(validationErrors)) {
      credentialFieldErrors = validationErrors;
      saving = false;
      return;
    }
    try {
      const body = {
        ...credentialForm,
        dbPort: Number(credentialForm.dbPort),
        enabled: Boolean(credentialForm.enabled),
      };
      const path = editingCredentialId
        ? `/admin/credentials/${encodeURIComponent(editingCredentialId)}`
        : "/admin/credentials";
      const method = editingCredentialId ? "PUT" : "POST";
      await apiRequest(config, path, { method, body: JSON.stringify(body) });
      toast = editingCredentialId
        ? $_("toast.credentialUpdated")
        : $_("toast.credentialCreated");
      clearCredentialEditor();
      await loadAdminCredentials();
    } catch (err) {
      if (!applyCredentialApiFieldErrors(err)) {
        setTranslatedError(err);
      }
    } finally {
      saving = false;
    }
  }

  function editCredential(item) {
    clearCredentialFieldErrors();
    editingCredentialId = item.id;
    credentialForm = { ...item, dbPassword: "" };
    resetCredentialTest();
  }

  async function testCredentialConnection() {
    error = null;
    clearCredentialConnectionFieldErrors();
    const validationErrors = validateCredentialConnectionForm();
    if (hasCredentialConnectionFieldErrors(validationErrors)) {
      credentialFieldErrors = validationErrors;
      resetCredentialTest();
      return;
    }

    credentialTest = { status: "testing" };
    try {
      const body = await apiRequest(config, "/admin/credentials/test", {
        method: "POST",
        body: JSON.stringify({
          existingCredentialId: editingCredentialId || "",
          dbHost: credentialForm.dbHost,
          dbPort: Number(credentialForm.dbPort),
          dbUser: credentialForm.dbUser,
          dbPassword: credentialForm.dbPassword,
        }),
      });
      credentialTest = { status: body.success ? "success" : "failure" };
    } catch (err) {
      if (!applyCredentialConnectionApiFieldErrors(err)) {
        setTranslatedError(err);
      }
      credentialTest = { status: "failure" };
    }
  }

  async function toggleCredentialEnabled(item) {
    saving = true;
    error = null;
    try {
      const updated = await apiRequest(
        config,
        `/admin/credentials/${encodeURIComponent(item.id)}`,
        {
          method: "PUT",
          body: JSON.stringify({
            ...item,
            enabled: !item.enabled,
          }),
        },
      );
      credentials = credentials.map((credential) =>
        credential.id === updated.id ? updated : credential,
      );
      if (editingCredentialId === updated.id) {
        credentialForm = { ...credentialForm, enabled: updated.enabled };
      }
      toast = updated.enabled
        ? $_("toast.credentialEnabled")
        : $_("toast.credentialDisabled");
    } catch (err) {
      setTranslatedError(err);
    } finally {
      saving = false;
    }
  }

  function requestDeleteCredential(item) {
    deleteDialog = {
      titleKey: "dialogs.deleteCredential.title",
      messageKey: "dialogs.deleteCredential.message",
      confirmKey: "dialogs.deleteCredential.confirm",
      values: { name: item.name, id: item.id },
      action: () => performDeleteCredential(item),
    };
  }

  function requestDeleteMapping(mapping) {
    deleteDialog = {
      titleKey: "dialogs.deleteMapping.title",
      messageKey: "dialogs.deleteMapping.message",
      confirmKey: "dialogs.deleteMapping.confirm",
      values: {
        subjectType: $_(subjectTypeKey(mapping.subjectType)),
        subject: mapping.subject,
        credentialId: mapping.credentialId,
      },
      action: () => performDeleteMapping(mapping),
    };
  }

  function closeDeleteDialog() {
    if (!deleteBusy) {
      deleteDialog = null;
    }
  }

  async function confirmDelete() {
    if (!deleteDialog) {
      return;
    }
    deleteBusy = true;
    error = null;
    try {
      await deleteDialog.action();
      deleteDialog = null;
    } catch (err) {
      setTranslatedError(err);
    } finally {
      deleteBusy = false;
    }
  }

  async function performDeleteCredential(item) {
    saving = true;
    try {
      await apiRequest(
        config,
        `/admin/credentials/${encodeURIComponent(item.id)}`,
        { method: "DELETE" },
      );
      if (editingCredentialId === item.id) {
        clearCredentialEditor();
      }
      toast = $_("toast.credentialDeleted");
      await loadAdminCredentials();
    } finally {
      saving = false;
    }
  }

  async function performDeleteMapping(mapping) {
    saving = true;
    try {
      await apiRequest(
        config,
        `/admin/mappings/${encodeURIComponent(mapping.id)}`,
        { method: "DELETE" },
      );
      toast = $_("toast.mappingDeleted");
      await loadMappings();
    } finally {
      saving = false;
    }
  }

  async function createMapping() {
    saving = true;
    error = null;
    clearMappingFieldErrors();
    const validationErrors = validateMappingForm();
    if (hasFieldErrors(validationErrors)) {
      mappingFieldErrors = validationErrors;
      saving = false;
      return;
    }
    try {
      await apiRequest(config, "/admin/mappings", {
        method: "POST",
        body: JSON.stringify(mappingForm),
      });
      mappingForm = blankMapping();
      toast = $_("toast.mappingCreated");
      await loadMappings();
    } catch (err) {
      if (!applyMappingApiFieldErrors(err)) {
        setTranslatedError(err);
      }
    } finally {
      saving = false;
    }
  }

  async function resetAudit() {
    resetBusy = true;
    error = null;
    try {
      await apiRequest(config, "/admin/audit-events/reset", {
        method: "POST",
        body: JSON.stringify({
          confirmation: resetConfirmation,
          reason: resetReason,
        }),
      });
      toast = $_("toast.auditReset");
      resetOpen = false;
      resetConfirmation = "";
      resetReason = "";
      await loadAudit(1);
    } catch (err) {
      setTranslatedError(err);
    } finally {
      resetBusy = false;
    }
  }

  function navigate(nextRoute) {
    route = nextRoute;
    mobileMenuOpen = false;
    localeMenuOpen = false;
    themeMenuOpen = false;
    aboutOpen = false;
    history.pushState({}, "", hrefFor(nextRoute));
    loadRoute();
  }

  function openAbout() {
    aboutOpen = true;
    mobileMenuOpen = false;
    localeMenuOpen = false;
    themeMenuOpen = false;
  }

  function closeAbout() {
    aboutOpen = false;
  }

  function currentRoute() {
    const path = window.location.pathname;
    const relative = path.startsWith(base)
      ? path.slice(base.length) || "/"
      : "/";
    if (relative.startsWith("/account")) return ROUTES.ACCOUNT;
    if (relative.startsWith("/admin/credentials"))
      return ROUTES.ADMIN_CREDENTIALS;
    if (relative.startsWith("/admin/mappings")) return ROUTES.ADMIN_MAPPINGS;
    if (relative.startsWith("/admin/audit")) return ROUTES.ADMIN_AUDIT;
    return ROUTES.DASHBOARD;
  }

  function hrefFor(nextRoute) {
    if (nextRoute === ROUTES.ACCOUNT) return `${base}/account`;
    if (nextRoute === ROUTES.ADMIN_CREDENTIALS)
      return `${base}/admin/credentials`;
    if (nextRoute === ROUTES.ADMIN_MAPPINGS) return `${base}/admin/mappings`;
    if (nextRoute === ROUTES.ADMIN_AUDIT) return `${base}/admin/audit`;
    return `${base}/`;
  }

  function toggleLocaleMenu() {
    localeMenuOpen = !localeMenuOpen;
    themeMenuOpen = false;
  }

  function toggleThemeMenu() {
    themeMenuOpen = !themeMenuOpen;
    localeMenuOpen = false;
  }

  function toggleSidebarCollapsed() {
    sidebarCollapsed = !sidebarCollapsed;
    writeSidebarCollapsed(sidebarCollapsed);
  }

  function selectLocale(localeCode) {
    setAppLocale(localeCode);
    localeMenuOpen = false;
  }

  function selectTheme(mode) {
    setThemeMode(mode);
    themeMenuOpen = false;
  }

  function navIcon(routeName) {
    return NAV_ITEM_ICONS[routeName] || "fa-solid fa-circle";
  }

  function themeMenuIcon() {
    if ($themePreference === "dark") return "fa-solid fa-moon";
    if ($themePreference === "light") return "fa-solid fa-sun";
    return "fa-solid fa-circle-half-stroke";
  }

  function pageTitle() {
    if (route === ROUTES.DASHBOARD) return $_("topbar.dashboardTitle");
    if (route === ROUTES.ACCOUNT) return $_("account.title");
    if (route === ROUTES.ADMIN_CREDENTIALS) return $_("nav.adminCredentials");
    if (route === ROUTES.ADMIN_MAPPINGS) return $_("nav.mappings");
    if (route === ROUTES.ADMIN_AUDIT) return $_("nav.auditLog");
    return $_("app.brandName");
  }

  function displayAppVersion() {
    const appVersion = config.version?.appVersion;
    if (!appVersion || appVersion === "unknown") {
      return $_("common.unknown");
    }
    return `v${appVersion}`;
  }

  $: aboutItems = [
    {
      label: $_("about.fields.gatewayVersion"),
      value: displayAppVersion(),
    },
    {
      label: $_("about.fields.phpMyAdminVersion"),
      value: config.version?.phpMyAdminVersion || $_("common.unknown"),
      monospace: true,
    },
    {
      label: $_("about.fields.buildCommit"),
      value: config.version?.appCommit || $_("common.unknown"),
      kind: "code",
      middle: false,
      lines: 2,
      copyable: Boolean(
        config.version?.appCommit && config.version.appCommit !== "unknown",
      ),
    },
  ];

  function clearCredentialEditor() {
    editingCredentialId = "";
    credentialForm = blankCredential();
    clearCredentialFieldErrors();
    resetCredentialTest();
  }

  function blankCredential() {
    return {
      id: "",
      name: "",
      dbHost: "",
      dbPort: 3306,
      dbUser: "",
      dbPassword: "",
      description: "",
      enabled: true,
    };
  }

  function blankMapping() {
    return {
      subjectType: SUBJECT_TYPE_OPTIONS[0],
      subject: "",
      credentialId: "",
    };
  }

  function blankCredentialTest() {
    return { status: "idle" };
  }

  function blankCredentialFieldErrors() {
    return {
      id: "",
      name: "",
      dbHost: "",
      dbPort: "",
      dbUser: "",
      dbPassword: "",
    };
  }

  function blankMappingFieldErrors() {
    return { subjectType: "", subject: "", credentialId: "" };
  }

  function blankAuditFilters() {
    return {
      actor: "",
      action: "",
      targetType: "",
      result: "",
      from: "",
      to: "",
      pageSize: 25,
    };
  }

  function blankAuditMetadata() {
    return { actions: [], targetTypes: [], actorSuggestions: [] };
  }

  function hasFieldErrors(fieldErrors) {
    return Object.values(fieldErrors).some(Boolean);
  }

  function hasCredentialConnectionFieldErrors(fieldErrors) {
    return Boolean(
      fieldErrors.dbHost ||
        fieldErrors.dbPort ||
        fieldErrors.dbUser ||
        fieldErrors.dbPassword,
    );
  }

  function clearCredentialFieldErrors() {
    credentialFieldErrors = blankCredentialFieldErrors();
  }

  function clearCredentialConnectionFieldErrors() {
    credentialFieldErrors = {
      ...credentialFieldErrors,
      dbHost: "",
      dbPort: "",
      dbUser: "",
      dbPassword: "",
    };
  }

  function clearMappingFieldErrors() {
    mappingFieldErrors = blankMappingFieldErrors();
  }

  function clearCredentialFieldError(field) {
    if (!credentialFieldErrors[field]) {
      return;
    }
    credentialFieldErrors = { ...credentialFieldErrors, [field]: "" };
  }

  function resetCredentialTest() {
    credentialTest = blankCredentialTest();
  }

  function onCredentialConnectionFieldInput(field) {
    clearCredentialFieldError(field);
    resetCredentialTest();
  }

  function clearMappingFieldError(field) {
    if (!mappingFieldErrors[field]) {
      return;
    }
    mappingFieldErrors = { ...mappingFieldErrors, [field]: "" };
  }

  function validateCredentialForm() {
    const fieldErrors = blankCredentialFieldErrors();
    const requirePassword = !editingCredentialId;
    if (!credentialForm.id.trim() && !editingCredentialId) {
      fieldErrors.id = requiredFieldError("credentialForm.fields.id");
    }
    if (!credentialForm.name.trim()) {
      fieldErrors.name = requiredFieldError("credentialForm.fields.name");
    }
    if (!credentialForm.dbHost.trim()) {
      fieldErrors.dbHost = requiredFieldError("credentialForm.fields.dbHost");
    }
    const dbPort = Number(credentialForm.dbPort);
    if (!Number.isInteger(dbPort) || dbPort < 1 || dbPort > 65535) {
      fieldErrors.dbPort = messageFieldError("errors.dbPortRange");
    }
    if (!credentialForm.dbUser.trim()) {
      fieldErrors.dbUser = requiredFieldError("credentialForm.fields.dbUser");
    }
    if (requirePassword && !credentialForm.dbPassword.trim()) {
      fieldErrors.dbPassword = requiredFieldError(
        "credentialForm.fields.password",
      );
    }
    return fieldErrors;
  }

  function validateCredentialConnectionForm() {
    const fieldErrors = {
      ...credentialFieldErrors,
      dbHost: "",
      dbPort: "",
      dbUser: "",
      dbPassword: "",
    };
    if (!credentialForm.dbHost.trim()) {
      fieldErrors.dbHost = requiredFieldError("credentialForm.fields.dbHost");
    }
    const dbPort = Number(credentialForm.dbPort);
    if (!Number.isInteger(dbPort) || dbPort < 1 || dbPort > 65535) {
      fieldErrors.dbPort = messageFieldError("errors.dbPortRange");
    }
    if (!credentialForm.dbUser.trim()) {
      fieldErrors.dbUser = requiredFieldError("credentialForm.fields.dbUser");
    }
    if (!editingCredentialId && !credentialForm.dbPassword.trim()) {
      fieldErrors.dbPassword = requiredFieldError(
        "credentialForm.fields.password",
      );
    }
    return fieldErrors;
  }

  function validateMappingForm() {
    const fieldErrors = blankMappingFieldErrors();
    if (!mappingForm.subject.trim()) {
      fieldErrors.subject = requiredFieldError("mappingForm.fields.subject");
    }
    if (!mappingForm.credentialId.trim()) {
      fieldErrors.credentialId = requiredFieldError(
        "mappingForm.fields.credential",
      );
    }
    return fieldErrors;
  }

  function applyCredentialApiFieldErrors(err) {
    const message = normalizeErrorMessage(err);
    const fieldErrors = blankCredentialFieldErrors();

    if (message === "invalid argument: dbPassword is required") {
      fieldErrors.dbPassword = requiredFieldError(
        "credentialForm.fields.password",
      );
    } else if (message === "invalid argument: id is required") {
      fieldErrors.id = requiredFieldError("credentialForm.fields.id");
    } else if (
      message === "invalid argument: dbPort must be between 1 and 65535"
    ) {
      fieldErrors.dbPort = messageFieldError("errors.dbPortRange");
    } else if (
      message === "invalid argument: id, name, dbHost, and dbUser are required"
    ) {
      if (!credentialForm.id.trim() && !editingCredentialId) {
        fieldErrors.id = requiredFieldError("credentialForm.fields.id");
      }
      if (!credentialForm.name.trim()) {
        fieldErrors.name = requiredFieldError("credentialForm.fields.name");
      }
      if (!credentialForm.dbHost.trim()) {
        fieldErrors.dbHost = requiredFieldError("credentialForm.fields.dbHost");
      }
      if (!credentialForm.dbUser.trim()) {
        fieldErrors.dbUser = requiredFieldError("credentialForm.fields.dbUser");
      }
    } else {
      return false;
    }

    credentialFieldErrors = fieldErrors;
    return hasFieldErrors(fieldErrors);
  }

  function applyCredentialConnectionApiFieldErrors(err) {
    const message = normalizeErrorMessage(err);
    const fieldErrors = {
      ...credentialFieldErrors,
      dbHost: "",
      dbPort: "",
      dbUser: "",
      dbPassword: "",
    };

    if (message === "invalid argument: dbHost is required") {
      fieldErrors.dbHost = requiredFieldError("credentialForm.fields.dbHost");
    } else if (
      message === "invalid argument: dbPort must be between 1 and 65535"
    ) {
      fieldErrors.dbPort = messageFieldError("errors.dbPortRange");
    } else if (message === "invalid argument: dbUser is required") {
      fieldErrors.dbUser = requiredFieldError("credentialForm.fields.dbUser");
    } else if (message === "invalid argument: dbPassword is required") {
      fieldErrors.dbPassword = requiredFieldError(
        "credentialForm.fields.password",
      );
    } else {
      return false;
    }

    credentialFieldErrors = fieldErrors;
    return hasCredentialConnectionFieldErrors(fieldErrors);
  }

  function applyMappingApiFieldErrors(err) {
    const message = normalizeErrorMessage(err);
    const fieldErrors = blankMappingFieldErrors();

    if (message === "invalid argument: subject and credentialId are required") {
      if (!mappingForm.subject.trim()) {
        fieldErrors.subject = requiredFieldError("mappingForm.fields.subject");
      }
      if (!mappingForm.credentialId.trim()) {
        fieldErrors.credentialId = requiredFieldError(
          "mappingForm.fields.credential",
        );
      }
    } else if (
      message === "invalid argument: subjectType must be user or group"
    ) {
      fieldErrors.subjectType = messageFieldError("errors.subjectTypeInvalid");
    } else {
      return false;
    }

    mappingFieldErrors = fieldErrors;
    return hasFieldErrors(fieldErrors);
  }

  function requiredFieldError(fieldKey) {
    return { kind: "requiredField", fieldKey };
  }

  function messageFieldError(messageKey, values = undefined) {
    return { kind: "message", messageKey, values };
  }

  function resolveFieldError(errorValue, _localeToken) {
    if (!errorValue) {
      return "";
    }
    if (typeof errorValue === "string") {
      return errorValue;
    }
    if (errorValue.kind === "requiredField") {
      return $_("common.validation.requiredField", {
        values: { field: $_(errorValue.fieldKey) },
      });
    }
    if (errorValue.kind === "message") {
      return $_(errorValue.messageKey, errorValue.values
        ? { values: errorValue.values }
        : undefined);
    }
    return "";
  }

  function normalizeErrorMessage(err) {
    return typeof err?.message === "string" ? err.message.trim() : "";
  }

  function topLevelErrorMessage(err, _localeToken) {
    if (!err) {
      return "";
    }
    const message = typeof err === "string" ? err : normalizeErrorMessage(err);
    const status =
      typeof err === "object" && err !== null ? (err.status ?? 0) : 0;
    return translateApiError(message, status);
  }

  function setTranslatedError(err) {
    error = {
      message: typeof err === "string" ? err : (err?.message ?? ""),
      status: typeof err === "object" && err !== null ? (err.status ?? 0) : 0,
    };
  }

  function subjectTypeKey(subjectType) {
    return `common.subjectType.${subjectType}`;
  }

  function resultKey(result) {
    return `common.results.${result}`;
  }

  function readSidebarCollapsed() {
    try {
      return window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === "1";
    } catch {
      return false;
    }
  }

  function writeSidebarCollapsed(value) {
    try {
      window.localStorage.setItem(
        SIDEBAR_COLLAPSED_STORAGE_KEY,
        value ? "1" : "0",
      );
    } catch {
      // Ignore persistence failures.
    }
  }
</script>

<div class:sidebar-collapsed={sidebarCollapsed} class="app-shell">
  <aside
    class:collapsed={sidebarCollapsed}
    class:open={mobileMenuOpen}
    class="sidebar"
  >
    <div class="sidebar-header">
      <div class="brand-mark" aria-hidden="true">
        <i class={BRAND_ICON_CLASS}></i>
      </div>
      <div class="sidebar-brand-copy">
        <div class="font-semibold">{$_("app.brandName")}</div>
        <div class="text-sm text-slate-500">{$_("app.brandSubtitle")}</div>
      </div>
    </div>
    <nav class="sidebar-nav grid gap-1">
      <button
        type="button"
        class:active={route === ROUTES.DASHBOARD}
        class="nav-link"
        aria-label={$_("nav.credentials")}
        title={$_("nav.credentials")}
        on:click={() => navigate(ROUTES.DASHBOARD)}
      >
        <i
          class={`nav-link-icon ${navIcon(ROUTES.DASHBOARD)}`}
          aria-hidden="true"
        ></i>
        <span class="nav-link-label">{$_("nav.credentials")}</span>
      </button>
      <button
        type="button"
        class:active={route === ROUTES.ACCOUNT}
        class="nav-link"
        aria-label={$_("nav.account")}
        title={$_("nav.account")}
        on:click={() => navigate(ROUTES.ACCOUNT)}
      >
        <i class={`nav-link-icon ${navIcon(ROUTES.ACCOUNT)}`} aria-hidden="true"
        ></i>
        <span class="nav-link-label">{$_("nav.account")}</span>
      </button>
      {#if me?.isAdmin}
        <button
          type="button"
          class:active={route === ROUTES.ADMIN_CREDENTIALS}
          class="nav-link"
          aria-label={$_("nav.adminCredentials")}
          title={$_("nav.adminCredentials")}
          on:click={() => navigate(ROUTES.ADMIN_CREDENTIALS)}
        >
          <i
            class={`nav-link-icon ${navIcon(ROUTES.ADMIN_CREDENTIALS)}`}
            aria-hidden="true"
          ></i>
          <span class="nav-link-label">{$_("nav.adminCredentials")}</span>
        </button>
        <button
          type="button"
          class:active={route === ROUTES.ADMIN_MAPPINGS}
          class="nav-link"
          aria-label={$_("nav.mappings")}
          title={$_("nav.mappings")}
          on:click={() => navigate(ROUTES.ADMIN_MAPPINGS)}
        >
          <i
            class={`nav-link-icon ${navIcon(ROUTES.ADMIN_MAPPINGS)}`}
            aria-hidden="true"
          ></i>
          <span class="nav-link-label">{$_("nav.mappings")}</span>
        </button>
        <button
          type="button"
          class:active={route === ROUTES.ADMIN_AUDIT}
          class="nav-link"
          aria-label={$_("nav.auditLog")}
          title={$_("nav.auditLog")}
          on:click={() => navigate(ROUTES.ADMIN_AUDIT)}
        >
          <i
            class={`nav-link-icon ${navIcon(ROUTES.ADMIN_AUDIT)}`}
            aria-hidden="true"
          ></i>
          <span class="nav-link-label">{$_("nav.auditLog")}</span>
        </button>
      {/if}
    </nav>
    <div class="sidebar-footer">
      <button
        type="button"
        class="sidebar-build sidebar-build-button"
        aria-label={$_("about.open")}
        title={$_("about.open")}
        on:click={openAbout}
      >
        <div class="sidebar-build-item">
          <span class="sidebar-build-label">{$_("app.phpMyAdminVersionLabel")}</span>
          <span class="sidebar-build-value" title={`phpMyAdmin ${config.version?.phpMyAdminVersion || $_("common.unknown")}`}>
            {config.version?.phpMyAdminVersion || $_("common.unknown")}
          </span>
        </div>
        <div class="sidebar-build-item">
          <span class="sidebar-build-label">{$_("app.gatewayVersionLabel")}</span>
          <span class="sidebar-build-value" title={displayAppVersion()}>
            {displayAppVersion()}
          </span>
        </div>
      </button>
      <button
        type="button"
        class="icon-button sidebar-collapse-button"
        aria-label={sidebarCollapsed
          ? $_("nav.expandSidebar")
          : $_("nav.collapseSidebar")}
        title={sidebarCollapsed
          ? $_("nav.expandSidebar")
          : $_("nav.collapseSidebar")}
        on:click={toggleSidebarCollapsed}
      >
        <span class="fluent-icon" aria-hidden="true">
          {@html sidebarCollapsed ? panelLeftExpandIcon : panelLeftContractIcon}
        </span>
      </button>
    </div>
  </aside>

  <main class="main">
    <header class="topbar">
      <div class="actions topbar-start">
        <button
          type="button"
          class="icon-button mobile-menu mobile-menu-button"
          aria-label={mobileMenuOpen ? $_("nav.closeMenu") : $_("nav.openMenu")}
          title={mobileMenuOpen ? $_("nav.closeMenu") : $_("nav.openMenu")}
          on:click={() => (mobileMenuOpen = !mobileMenuOpen)}
        >
          {#if mobileMenuOpen}
            <i class="fa-solid fa-xmark" aria-hidden="true"></i>
          {:else}
            <i class="fa-solid fa-bars" aria-hidden="true"></i>
          {/if}
        </button>
        <div class="topbar-title">
          <div class="topbar-title-main">{pageTitle()}</div>
        </div>
      </div>
      <div class="topbar-end">
        <div class="theme-menu">
          <button
            type="button"
            class="icon-button topbar-icon-button"
            aria-haspopup="menu"
            aria-expanded={themeMenuOpen ? "true" : "false"}
            aria-label={themeMenuOpen
              ? $_("common.closeThemeMenu")
              : $_("common.openThemeMenu")}
            title={$_("common.themeLabel")}
            on:click|stopPropagation={toggleThemeMenu}
          >
            <i class={themeMenuIcon()} aria-hidden="true"></i>
          </button>
          {#if themeMenuOpen}
            <div
              class="dropdown-menu"
              role="menu"
              aria-label={$_("common.themeLabel")}
            >
              {#each supportedThemeModes as mode}
                <button
                  type="button"
                  class:active={$themePreference === mode}
                  class="dropdown-item"
                  role="menuitemradio"
                  aria-checked={$themePreference === mode}
                  on:click={() => selectTheme(mode)}
                >
                  <span>{$_(`common.themeModes.${mode}`)}</span>
                  {#if $themePreference === mode}
                    <i class="fa-solid fa-check" aria-hidden="true"></i>
                  {/if}
                </button>
              {/each}
            </div>
          {/if}
        </div>
        <div class="locale-menu">
          <button
            type="button"
            class="icon-button topbar-icon-button"
            aria-haspopup="menu"
            aria-expanded={localeMenuOpen ? "true" : "false"}
            aria-label={localeMenuOpen
              ? $_("common.closeLanguageMenu")
              : $_("common.openLanguageMenu")}
            title={$_("common.localeLabel")}
            on:click|stopPropagation={toggleLocaleMenu}
          >
            <i class="fa-solid fa-language" aria-hidden="true"></i>
          </button>
          {#if localeMenuOpen}
            <div
              class="dropdown-menu"
              role="menu"
              aria-label={$_("common.localeLabel")}
            >
              {#each supportedLocales as localeCode}
                <button
                  type="button"
                  class:active={$locale === localeCode}
                  class="dropdown-item"
                  role="menuitemradio"
                  aria-checked={$locale === localeCode}
                  on:click={() => selectLocale(localeCode)}
                >
                  <span>{$_(`common.locales.long.${localeCode}`)}</span>
                  {#if $locale === localeCode}
                    <i class="fa-solid fa-check" aria-hidden="true"></i>
                  {/if}
                </button>
              {/each}
            </div>
          {/if}
        </div>
        <button
          type="button"
          class:active={route === ROUTES.ACCOUNT}
          class="user-summary"
          aria-label={$_("account.open")}
          on:click={() => navigate(ROUTES.ACCOUNT)}
        >
          <i class="fa-solid fa-user" aria-hidden="true"></i>
          <span>{me?.user || $_("topbar.loadingUser")}</span>
        </button>
      </div>
    </header>

    <section class:content--narrow={route === ROUTES.ACCOUNT} class="content">
      {#if loading}
        <div class="panel">
          <div class="panel-body">{$_("common.loading")}</div>
        </div>
      {:else if error}
        <div class="alert alert--error mb-4" role="alert">
          <div class="alert-icon">
            <i class="fa-solid fa-circle-exclamation" aria-hidden="true"></i>
          </div>
          <div class="alert-body">
            <div class="alert-label">{$_("common.labels.error")}</div>
            <div class="alert-message">
              {topLevelErrorMessage(error, $locale)}
            </div>
          </div>
        </div>
      {/if}

      {#if !loading && route === ROUTES.DASHBOARD}
        <div class="grid-list">
          {#each credentials as item}
            <button
              type="button"
              class="credential-card"
              disabled={saving}
              aria-label={$_("dashboard.openCredentialAria", {
                values: { name: item.name },
              })}
              on:click={() => startSession(item.id)}
            >
              <div>
                <h2 class="text-lg font-semibold">{item.name}</h2>
                <p class="text-sm text-slate-500">
                  {item.description || item.id}
                </p>
              </div>
              <div class="text-sm">
                <div>{item.dbUser}@{item.dbHost}:{item.dbPort}</div>
              </div>
              <div class="credential-card-footer">
                <span class={item.enabled ? "badge success" : "badge"}>
                  {item.enabled
                    ? $_("common.status.enabled")
                    : $_("common.status.disabled")}
                </span>
                <span class="open-hint"
                  >{saving
                    ? $_("common.opening")
                    : $_("common.openPhpMyAdmin")}</span
                >
              </div>
            </button>
          {:else}
            <div class="panel">
              <div class="panel-body">{$_("dashboard.empty")}</div>
            </div>
          {/each}
        </div>
      {/if}

      {#if !loading && route === ROUTES.ACCOUNT}
        <div class="panel">
          <div class="panel-header">
            <h2 class="font-semibold">{$_("account.title")}</h2>
          </div>
          <div class="panel-body">
            <div class="account-grid">
              <section class="account-section">
                <h3 class="account-section-title">
                  {$_("account.sections.identity")}
                </h3>
                <div class="account-user">{me?.user}</div>
              </section>
              <section class="account-section">
                <div class="account-section-header">
                  <h3 class="account-section-title">
                    {$_("account.sections.groups")}
                  </h3>
                  <span class="text-sm text-slate-500"
                    >{$_("account.groupCount", {
                      values: { count: me?.groups?.length || 0 },
                    })}</span
                  >
                </div>
                {#if me?.groups?.length}
                  <div class="account-group-list">
                    {#each me.groups as group}
                      <span class="badge">{group}</span>
                    {/each}
                  </div>
                {:else}
                  <div class="text-sm text-slate-500">
                    {$_("account.emptyGroups")}
                  </div>
                {/if}
              </section>
            </div>
          </div>
        </div>
      {/if}

      {#if !loading && route === ROUTES.ADMIN_CREDENTIALS && me?.isAdmin}
        <div class="panel mb-4">
          <div class="panel-header">
            <h2 class="font-semibold">
              {editingCredentialId
                ? $_("credentialForm.editTitle")
                : $_("credentialForm.createTitle")}
            </h2>
            {#if editingCredentialId}
              <button
                type="button"
                class="button secondary"
                on:click={clearCredentialEditor}>{$_("common.cancel")}</button
              >
            {/if}
          </div>
          <div class="panel-body">
            <div class="credential-form-sections">
              <fieldset class="form-section">
                <legend class="form-section-title"
                  >{$_("credentialForm.groups.identity")}</legend
                >
                <div class="form-grid credential-form-grid">
                  <label class="field">
                    <span>{$_("credentialForm.fields.id")}</span>
                    <input
                      bind:value={credentialForm.id}
                      disabled={Boolean(editingCredentialId)}
                      class:field-control--invalid={Boolean(
                        credentialFieldErrors.id,
                      )}
                      aria-invalid={credentialFieldErrors.id ? "true" : "false"}
                      aria-describedby={credentialFieldErrors.id
                        ? "credential-id-error"
                        : undefined}
                      on:input={() => clearCredentialFieldError("id")}
                    />
                    {#if credentialFieldErrors.id}
                      <div id="credential-id-error" class="field-error">
                        {resolveFieldError(credentialFieldErrors.id, $locale)}
                      </div>
                    {/if}
                  </label>
                  <label class="field">
                    <span>{$_("credentialForm.fields.name")}</span>
                    <input
                      bind:value={credentialForm.name}
                      class:field-control--invalid={Boolean(
                        credentialFieldErrors.name,
                      )}
                      aria-invalid={credentialFieldErrors.name
                        ? "true"
                        : "false"}
                      aria-describedby={credentialFieldErrors.name
                        ? "credential-name-error"
                        : undefined}
                      on:input={() => clearCredentialFieldError("name")}
                    />
                    {#if credentialFieldErrors.name}
                      <div id="credential-name-error" class="field-error">
                        {resolveFieldError(credentialFieldErrors.name, $locale)}
                      </div>
                    {/if}
                  </label>
                  <label class="field credential-form-wide">
                    <span>{$_("credentialForm.fields.description")}</span>
                    <input bind:value={credentialForm.description} />
                  </label>
                </div>
              </fieldset>

              <fieldset class="form-section">
                <legend class="form-section-title"
                  >{$_("credentialForm.groups.database")}</legend
                >
                <div class="form-grid credential-form-grid">
                  <label class="field">
                    <span>{$_("credentialForm.fields.dbHost")}</span>
                    <input
                      bind:value={credentialForm.dbHost}
                      class:field-control--invalid={Boolean(
                        credentialFieldErrors.dbHost,
                      )}
                      aria-invalid={credentialFieldErrors.dbHost
                        ? "true"
                        : "false"}
                      aria-describedby={credentialFieldErrors.dbHost
                        ? "credential-dbhost-error"
                        : undefined}
                      on:input={() =>
                        onCredentialConnectionFieldInput("dbHost")}
                    />
                    {#if credentialFieldErrors.dbHost}
                      <div id="credential-dbhost-error" class="field-error">
                        {resolveFieldError(credentialFieldErrors.dbHost, $locale)}
                      </div>
                    {/if}
                  </label>
                  <label class="field">
                    <span>{$_("credentialForm.fields.dbPort")}</span>
                    <input
                      type="number"
                      min="1"
                      max="65535"
                      bind:value={credentialForm.dbPort}
                      class:field-control--invalid={Boolean(
                        credentialFieldErrors.dbPort,
                      )}
                      aria-invalid={credentialFieldErrors.dbPort
                        ? "true"
                        : "false"}
                      aria-describedby={credentialFieldErrors.dbPort
                        ? "credential-dbport-error"
                        : undefined}
                      on:input={() =>
                        onCredentialConnectionFieldInput("dbPort")}
                    />
                    {#if credentialFieldErrors.dbPort}
                      <div id="credential-dbport-error" class="field-error">
                        {resolveFieldError(credentialFieldErrors.dbPort, $locale)}
                      </div>
                    {/if}
                  </label>
                  <label class="field">
                    <span>{$_("credentialForm.fields.dbUser")}</span>
                    <input
                      bind:value={credentialForm.dbUser}
                      class:field-control--invalid={Boolean(
                        credentialFieldErrors.dbUser,
                      )}
                      aria-invalid={credentialFieldErrors.dbUser
                        ? "true"
                        : "false"}
                      aria-describedby={credentialFieldErrors.dbUser
                        ? "credential-dbuser-error"
                        : undefined}
                      on:input={() =>
                        onCredentialConnectionFieldInput("dbUser")}
                    />
                    {#if credentialFieldErrors.dbUser}
                      <div id="credential-dbuser-error" class="field-error">
                        {resolveFieldError(credentialFieldErrors.dbUser, $locale)}
                      </div>
                    {/if}
                  </label>
                  <label class="field">
                    <span>{$_("credentialForm.fields.password")}</span>
                    <input
                      type="password"
                      bind:value={credentialForm.dbPassword}
                      placeholder={editingCredentialId
                        ? $_("credentialForm.passwordHint")
                        : ""}
                      class:field-control--invalid={Boolean(
                        credentialFieldErrors.dbPassword,
                      )}
                      aria-invalid={credentialFieldErrors.dbPassword
                        ? "true"
                        : "false"}
                      aria-describedby={credentialFieldErrors.dbPassword
                        ? "credential-dbpassword-error"
                        : undefined}
                      on:input={() =>
                        onCredentialConnectionFieldInput("dbPassword")}
                    />
                    {#if credentialFieldErrors.dbPassword}
                      <div id="credential-dbpassword-error" class="field-error">
                        {resolveFieldError(
                          credentialFieldErrors.dbPassword,
                          $locale,
                        )}
                      </div>
                    {/if}
                  </label>
                </div>
              </fieldset>
            </div>
            <div class="actions mt-4">
              <button
                type="button"
                class="button secondary"
                disabled={saving || credentialTest.status === "testing"}
                on:click={testCredentialConnection}
              >
                {credentialTest.status === "testing"
                  ? $_("common.testing")
                  : $_("common.testConnection")}
              </button>
              <button
                type="button"
                class="button"
                disabled={saving || credentialTest.status === "testing"}
                on:click={saveCredential}
              >
                {editingCredentialId
                  ? $_("common.update")
                  : $_("common.create")}
              </button>
            </div>
            <div class="credential-test-feedback" aria-live="polite">
              {#if credentialTest.status === "testing"}
                <div class="async-status async-status--testing">
                  <i class="fa-solid fa-spinner fa-spin" aria-hidden="true"></i>
                  <span>{$_("credentialForm.connectionTest.testing")}</span>
                </div>
              {:else if credentialTest.status === "success"}
                <div class="async-status async-status--success">
                  <i class="fa-solid fa-circle-check" aria-hidden="true"></i>
                  <span>{$_("credentialForm.connectionTest.success")}</span>
                </div>
              {:else if credentialTest.status === "failure"}
                <div class="async-status async-status--failure">
                  <i class="fa-solid fa-circle-xmark" aria-hidden="true"></i>
                  <span>{$_("credentialForm.connectionTest.failure")}</span>
                </div>
              {:else}
                <div class="async-status async-status--idle">
                  <span>{$_("credentialForm.connectionTest.idle")}</span>
                </div>
              {/if}
            </div>
          </div>
        </div>
        <CredentialTable
          {credentials}
          onEdit={editCredential}
          onToggleEnabled={toggleCredentialEnabled}
          onDelete={requestDeleteCredential}
          disabled={saving || deleteBusy}
        />
      {/if}

      {#if !loading && route === ROUTES.ADMIN_MAPPINGS && me?.isAdmin}
        <div class="panel mb-4">
          <div class="panel-header">
            <h2 class="font-semibold">{$_("mappingForm.createTitle")}</h2>
          </div>
          <div class="panel-body">
            <div class="form-grid">
              <label class="field">
                <span>{$_("mappingForm.fields.subjectType")}</span>
                <select
                  bind:value={mappingForm.subjectType}
                  class:field-control--invalid={Boolean(
                    mappingFieldErrors.subjectType,
                  )}
                  aria-invalid={mappingFieldErrors.subjectType
                    ? "true"
                    : "false"}
                  aria-describedby={mappingFieldErrors.subjectType
                    ? "mapping-subjecttype-error"
                    : undefined}
                  on:change={() => clearMappingFieldError("subjectType")}
                >
                  {#each SUBJECT_TYPE_OPTIONS as subjectType}
                    <option value={subjectType}
                      >{$_(subjectTypeKey(subjectType))}</option
                    >
                  {/each}
                </select>
                {#if mappingFieldErrors.subjectType}
                  <div id="mapping-subjecttype-error" class="field-error">
                    {resolveFieldError(mappingFieldErrors.subjectType, $locale)}
                  </div>
                {/if}
              </label>
              <label class="field">
                <span>{$_("mappingForm.fields.subject")}</span>
                <input
                  bind:value={mappingForm.subject}
                  class:field-control--invalid={Boolean(
                    mappingFieldErrors.subject,
                  )}
                  aria-invalid={mappingFieldErrors.subject ? "true" : "false"}
                  aria-describedby={mappingFieldErrors.subject
                    ? "mapping-subject-error"
                    : undefined}
                  on:input={() => clearMappingFieldError("subject")}
                />
                {#if mappingFieldErrors.subject}
                  <div id="mapping-subject-error" class="field-error">
                    {resolveFieldError(mappingFieldErrors.subject, $locale)}
                  </div>
                {/if}
              </label>
              <label class="field">
                <span>{$_("mappingForm.fields.credential")}</span>
                <select
                  bind:value={mappingForm.credentialId}
                  class:field-control--invalid={Boolean(
                    mappingFieldErrors.credentialId,
                  )}
                  aria-invalid={mappingFieldErrors.credentialId
                    ? "true"
                    : "false"}
                  aria-describedby={mappingFieldErrors.credentialId
                    ? "mapping-credential-error"
                    : undefined}
                  on:change={() => clearMappingFieldError("credentialId")}
                >
                  <option value="">{$_("mappingForm.selectCredential")}</option>
                  {#each credentials as credential}
                    <option value={credential.id}>{credential.name}</option>
                  {/each}
                </select>
                {#if mappingFieldErrors.credentialId}
                  <div id="mapping-credential-error" class="field-error">
                    {resolveFieldError(mappingFieldErrors.credentialId, $locale)}
                  </div>
                {/if}
              </label>
            </div>
            <div class="actions mt-4">
              <button
                type="button"
                class="button"
                disabled={saving}
                on:click={createMapping}>{$_("common.create")}</button
              >
            </div>
          </div>
        </div>
        <MappingList
          {mappings}
          onDelete={requestDeleteMapping}
          disabled={saving || deleteBusy}
        />
      {/if}

      {#if !loading && route === ROUTES.ADMIN_AUDIT && me?.isAdmin}
        <div class="panel mb-4">
          <div class="panel-header">
            <h2 class="font-semibold">{$_("audit.title")}</h2>
            <div class="actions">
              <button
                type="button"
                class="button secondary"
                on:click={() => loadAudit(auditPage.page)}
              >
                {$_("common.refresh")}
              </button>
              <button
                type="button"
                class="button danger"
                on:click={() => (resetOpen = true)}>{$_("common.reset")}</button
              >
            </div>
          </div>
          <div class="panel-body">
            <div class="form-grid">
              <label class="field">
                <span>{$_("audit.filters.actor")}</span>
                <input
                  bind:value={auditFilters.actor}
                  list={AUDIT_ACTOR_SUGGESTIONS_ID}
                  autocomplete="off"
                />
                <datalist id={AUDIT_ACTOR_SUGGESTIONS_ID}>
                  {#each auditMetadata.actorSuggestions as actor}
                    <option value={actor}></option>
                  {/each}
                </datalist>
              </label>
              <label class="field">
                <span>{$_("audit.filters.action")}</span>
                <select bind:value={auditFilters.action}>
                  <option value="">{$_("common.any")}</option>
                  {#each auditMetadata.actions as action}
                    <option value={action}
                      >{translateAuditAction(action, $locale)}</option
                    >
                  {/each}
                </select>
              </label>
              <label class="field">
                <span>{$_("audit.filters.targetType")}</span>
                <select bind:value={auditFilters.targetType}>
                  <option value="">{$_("common.any")}</option>
                  {#each auditMetadata.targetTypes as targetType}
                    <option value={targetType}
                      >{translateAuditTargetType(targetType, $locale)}</option
                    >
                  {/each}
                </select>
              </label>
              <label class="field">
                <span>{$_("audit.filters.result")}</span>
                <select bind:value={auditFilters.result}>
                  {#each AUDIT_RESULT_OPTIONS as resultOption}
                    <option value={resultOption}>
                      {resultOption
                        ? $_(resultKey(resultOption))
                        : $_("common.any")}
                    </option>
                  {/each}
                </select>
              </label>
              <label class="field"
                ><span>{$_("audit.filters.from")}</span><input
                  type="datetime-local"
                  bind:value={auditFilters.from}
                /></label
              >
              <label class="field"
                ><span>{$_("audit.filters.to")}</span><input
                  type="datetime-local"
                  bind:value={auditFilters.to}
                /></label
              >
              <label class="field">
                <span>{$_("audit.filters.pageSize")}</span>
                <select bind:value={auditFilters.pageSize}>
                  {#each AUDIT_PAGE_SIZES as pageSize}
                    <option value={pageSize}>{pageSize}</option>
                  {/each}
                </select>
              </label>
            </div>
            <div class="actions mt-4">
              <button type="button" class="button" on:click={() => loadAudit(1)}
                >{$_("common.apply")}</button
              >
            </div>
          </div>
        </div>
        <AuditLogList
          {auditPage}
          onPrevious={() => loadAudit(auditPage.page - 1)}
          onNext={() => loadAudit(auditPage.page + 1)}
        />
      {/if}
    </section>
  </main>
</div>

{#if toast}
  <button type="button" class="toast text-left" on:click={() => (toast = "")}
    >{toast}</button
  >
{/if}

{#if aboutOpen}
  <div class="modal-backdrop">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="about-dialog-title"
      tabindex="-1"
    >
      <div class="panel-header">
        <h2 id="about-dialog-title" class="font-semibold">
          {$_("about.title")}
        </h2>
      </div>
      <div class="panel-body grid gap-4">
        <p class="modal-description">
          {$_("about.description")}
        </p>
        <KeyValueList items={aboutItems} />
        <div class="actions">
          <button
            type="button"
            class="button secondary"
            on:click={closeAbout}
          >
            {$_("common.close")}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

{#if deleteDialog}
  <div class="modal-backdrop">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-dialog-title"
    >
      <div class="panel-header">
        <h2 id="delete-dialog-title" class="font-semibold">
          {$_(deleteDialog.titleKey)}
        </h2>
      </div>
      <div class="panel-body grid gap-3">
        <div class="alert alert--error" role="alert">
          <div class="alert-icon">
            <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
          </div>
          <div class="alert-body">
            <div class="alert-label">{$_("common.labels.danger")}</div>
            <div>
              {$_(deleteDialog.messageKey, { values: deleteDialog.values })}
            </div>
          </div>
        </div>
        <div class="actions">
          <button
            type="button"
            class="button danger"
            disabled={deleteBusy}
            on:click={confirmDelete}
          >
            {deleteBusy ? $_("common.deleting") : $_(deleteDialog.confirmKey)}
          </button>
          <button
            type="button"
            class="button secondary"
            disabled={deleteBusy}
            on:click={closeDeleteDialog}
          >
            {$_("common.cancel")}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

{#if resetOpen}
  <div class="modal-backdrop">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="reset-dialog-title"
    >
      <div class="panel-header">
        <h2 id="reset-dialog-title" class="font-semibold">
          {$_("audit.reset.title")}
        </h2>
      </div>
      <div class="panel-body grid gap-3">
        <div class="alert alert--warning" role="alert">
          <div class="alert-icon">
            <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
          </div>
          <div class="alert-body">
            <div class="alert-label">{$_("common.labels.warning")}</div>
            <div>{$_("audit.reset.description")}</div>
          </div>
        </div>
        <label class="field">
          <span>{$_("audit.reset.confirmation")}</span>
          <input
            bind:value={resetConfirmation}
            placeholder={$_("audit.reset.confirmationPlaceholder")}
          />
        </label>
        <label class="field">
          <span>{$_("audit.reset.reason")}</span>
          <textarea rows="3" bind:value={resetReason}></textarea>
        </label>
        <div class="actions">
          <button
            type="button"
            class="button danger"
            disabled={resetBusy ||
              resetConfirmation !== AUDIT_RESET_CONFIRMATION}
            on:click={resetAudit}
          >
            {resetBusy ? $_("common.resetting") : $_("common.reset")}
          </button>
          <button
            type="button"
            class="button secondary"
            disabled={resetBusy}
            on:click={() => (resetOpen = false)}
          >
            {$_("common.cancel")}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
