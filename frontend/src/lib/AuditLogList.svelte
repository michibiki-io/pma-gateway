<script>
  import {
    _,
    locale,
    translateAuditAction,
    translateAuditMessage,
    translateAuditTargetType,
  } from "./i18n/index.js";
  import AuditLogCard from "./AuditLogCard.svelte";
  import CodeValue from "./CodeValue.svelte";
  import TruncatedText from "./TruncatedText.svelte";

  export let auditPage = {
    items: [],
    page: 1,
    totalPages: 1,
  };
  export let onPrevious = () => {};
  export let onNext = () => {};

  function resultClass(result) {
    if (result === "success") return "badge success";
    if (result === "denied") return "badge denied";
    return "badge failure";
  }

  function resultKey(result) {
    return `common.results.${result}`;
  }

  function formatAuditTarget(event, _localeToken) {
    const targetType = translateAuditTargetType(event.targetType, $locale);
    if (!event.targetId) {
      return { label: targetType, value: "", hasValue: false };
    }
    return { label: targetType, value: event.targetId, hasValue: true };
  }
</script>

<div class="panel">
  <div class="table-wrap desktop-data-table">
    <table class="table-middle">
      <thead>
        <tr>
          <th>{$_("tables.headers.timestamp")}</th>
          <th>{$_("tables.headers.actor")}</th>
          <th>{$_("tables.headers.action")}</th>
          <th>{$_("tables.headers.target")}</th>
          <th>{$_("tables.headers.result")}</th>
          <th>{$_("tables.headers.remote")}</th>
          <th>{$_("tables.headers.message")}</th>
        </tr>
      </thead>
      <tbody>
        {#each auditPage.items as event}
          <tr>
            <td><TruncatedText value={event.timestamp} lines={1} /></td>
            <td><TruncatedText value={event.actor} lines={1} /></td>
            <td
              ><TruncatedText
                value={translateAuditAction(event.action, $locale)}
                lines={2}
              /></td
            >
            <td>
              {#if formatAuditTarget(event, $locale).hasValue}
                <div class="audit-target-cell">
                  <span class="audit-target-label">
                    {formatAuditTarget(event, $locale).label}
                  </span>
                  <CodeValue
                    label={formatAuditTarget(event, $locale).label}
                    value={formatAuditTarget(event, $locale).value}
                    copyable={false}
                  />
                </div>
              {:else}
                <TruncatedText
                  value={formatAuditTarget(event, $locale).label}
                  lines={1}
                />
              {/if}
            </td>
            <td>
              <span class={resultClass(event.result)}
                >{$_(resultKey(event.result))}</span
              >
            </td>
            <td><TruncatedText value={event.remoteAddress} lines={1} /></td>
            <td
              ><TruncatedText
                value={translateAuditMessage(event.message, $locale)}
                lines={2}
              /></td
            >
          </tr>
        {:else}
          <tr><td colspan="7">{$_("tables.emptyAudit")}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="mobile-data-list">
    {#if auditPage.items.length}
      {#each auditPage.items as event (event.id || `${event.timestamp}-${event.actor}-${event.action}`)}
        <AuditLogCard {event} />
      {/each}
    {:else}
      <div class="panel-body">{$_("tables.emptyAudit")}</div>
    {/if}
  </div>

  <div class="panel-body audit-pagination">
    <button
      type="button"
      class="button secondary"
      aria-label={$_("common.previous")}
      disabled={auditPage.page <= 1}
      on:click={onPrevious}
    >
      {$_("common.previous")}
    </button>
    <span class="audit-page-indicator">
      {$_("audit.pageIndicator", {
        values: {
          page: auditPage.page,
          total: auditPage.totalPages || 1,
        },
      })}
    </span>
    <button
      type="button"
      class="button secondary"
      aria-label={$_("common.next")}
      disabled={auditPage.page >= (auditPage.totalPages || 1)}
      on:click={onNext}
    >
      {$_("common.next")}
    </button>
  </div>
</div>
