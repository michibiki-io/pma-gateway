<script>
  import {
    _,
    locale,
    translateAuditAction,
    translateAuditMessage,
    translateAuditTargetType,
  } from "./i18n/index.js";
  import KeyValueList from "./KeyValueList.svelte";
  import TruncatedText from "./TruncatedText.svelte";
  import {
    extractAuditSummaryItems,
    stringifyAuditMetadata,
  } from "./audit-log.js";

  export let event;

  let expanded = false;

  function resultClass(result) {
    if (result === "success") return "badge success";
    if (result === "denied") return "badge denied";
    return "badge failure";
  }

  function resultKey(result) {
    return `common.results.${result}`;
  }

  function summaryLabel(item, targetType, _localeToken) {
    if (item.key === "targetId") {
      return targetType || $_("audit.details.targetId");
    }
    if (item.key === "credentialId") {
      return $_("tables.headers.credential");
    }
    if (item.key === "subject") {
      return $_("tables.headers.subject");
    }
    if (item.key === "count") {
      return $_("audit.mobile.count");
    }
    return item.key;
  }

  function detailItemsFor(event, targetType, _localeToken) {
    const items = [];
    if (event?.id) {
      items.push({
        label: $_("audit.details.eventId"),
        value: event.id,
        kind: "code",
        copyable: true,
      });
    }
    if (event?.targetId) {
      items.push({
        label: targetType || $_("audit.details.targetId"),
        value: event.targetId,
        kind: "code",
        copyable: true,
      });
    }
    if (event?.actorGroups?.length) {
      items.push({
        label: $_("audit.details.actorGroups"),
        value: event.actorGroups.join(", "),
        lines: 2,
      });
    }
    if (event?.remoteAddress) {
      items.push({
        label: $_("tables.headers.remote"),
        value: event.remoteAddress,
        lines: 2,
      });
    }
    if (event?.userAgent) {
      items.push({
        label: $_("audit.details.userAgent"),
        value: event.userAgent,
        lines: 3,
      });
    }
    return items;
  }

  $: translatedAction = translateAuditAction(event?.action, $locale);
  $: translatedMessage = translateAuditMessage(event?.message, $locale);
  $: translatedTargetType = translateAuditTargetType(
    event?.targetType,
    $locale,
  );
  $: summaryItems = extractAuditSummaryItems(event).map((item) => ({
    label: summaryLabel(item, translatedTargetType, $locale),
    value: item.value,
    kind: item.kind,
    copyable: item.kind === "code",
    lines: item.kind === "code" ? 1 : 2,
  }));
  $: detailItems = detailItemsFor(event, translatedTargetType, $locale);
  $: metadataText = stringifyAuditMetadata(event?.metadata);
  $: hasDetails = detailItems.length > 0 || Boolean(metadataText);
</script>

<article class="data-card audit-data-card">
  <div class="data-card-header data-card-header--stack">
    <div class="audit-card-meta">
      <time class="audit-card-timestamp">{event.timestamp}</time>
      <div class="audit-card-actor">
        <TruncatedText value={event.actor} lines={1} />
      </div>
    </div>
    <div class="audit-card-badges">
      <span class="badge">{translatedAction}</span>
      <span class={resultClass(event.result)}
        >{$_(resultKey(event.result))}</span
      >
    </div>
  </div>

  <div class="data-card-body">
    <div class="audit-card-summary">
      <TruncatedText
        value={translatedMessage}
        lines={3}
        className="audit-card-summary-text"
      />
    </div>

    {#if summaryItems.length}
      <KeyValueList items={summaryItems} dense={true} />
    {/if}

    {#if hasDetails}
      <div class="audit-card-details-toggle">
        <button
          type="button"
          class="button secondary"
          aria-label={expanded
            ? $_("audit.mobile.hideDetails")
            : $_("audit.mobile.showDetails")}
          aria-expanded={expanded ? "true" : "false"}
          on:click={() => (expanded = !expanded)}
        >
          {expanded
            ? $_("audit.mobile.hideDetails")
            : $_("audit.mobile.showDetails")}
        </button>
      </div>
    {/if}

    {#if expanded}
      <div class="audit-card-details">
        {#if detailItems.length}
          <KeyValueList items={detailItems} dense={true} />
        {/if}

        {#if metadataText}
          <div class="audit-raw-block">
            <div class="audit-raw-label">
              {$_("audit.details.metadataJson")}
            </div>
            <pre>{metadataText}</pre>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</article>
