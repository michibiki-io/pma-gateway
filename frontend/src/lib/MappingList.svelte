<script>
  import { _ } from "./i18n/index.js";
  import CodeValue from "./CodeValue.svelte";
  import KeyValueList from "./KeyValueList.svelte";
  import TruncatedText from "./TruncatedText.svelte";

  export let mappings = [];
  export let onDelete = () => {};
  export let disabled = false;

  function subjectTypeKey(subjectType) {
    return `common.subjectType.${subjectType}`;
  }

  function mobileItems(mapping) {
    return [
      {
        label: $_("tables.headers.credential"),
        value: mapping.credentialId,
        kind: "code",
        copyable: true,
      },
    ];
  }
</script>

<div class="panel">
  <div class="panel-header">
    <h2 class="font-semibold">{$_("tables.mappingsTitle")}</h2>
  </div>

  <div class="table-wrap desktop-data-table">
    <table class="table-middle">
      <thead>
        <tr>
          <th>{$_("tables.headers.subject")}</th>
          <th>{$_("tables.headers.credential")}</th>
          <th><span class="sr-only">{$_("tables.headers.actions")}</span></th>
        </tr>
      </thead>
      <tbody>
        {#each mappings as mapping}
          <tr>
            <td>
              <div class="mapping-subject-cell">
                <span class="badge"
                  >{$_(subjectTypeKey(mapping.subjectType))}</span
                >
                <TruncatedText value={mapping.subject} lines={1} />
              </div>
            </td>
            <td>
              <CodeValue
                label={$_("tables.headers.credential")}
                value={mapping.credentialId}
                copyable={false}
              />
            </td>
            <td>
              <div class="row-actions row-actions--inline">
                <button
                  type="button"
                  class="button danger"
                  {disabled}
                  on:click={() => onDelete(mapping)}
                >
                  {$_("common.delete")}
                </button>
              </div>
            </td>
          </tr>
        {:else}
          <tr><td colspan="3">{$_("tables.emptyMappings")}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="mobile-data-list">
    {#if mappings.length}
      {#each mappings as mapping}
        <article class="data-card">
          <div class="data-card-header">
            <span class="badge">{$_(subjectTypeKey(mapping.subjectType))}</span>
          </div>
          <div class="data-card-body">
            <div class="data-card-title">
              <TruncatedText value={mapping.subject} lines={2} />
            </div>
            <KeyValueList items={mobileItems(mapping)} dense={true} />
          </div>
          <div class="row-actions row-actions--mobile">
            <button
              type="button"
              class="button danger"
              {disabled}
              on:click={() => onDelete(mapping)}
            >
              {$_("common.delete")}
            </button>
          </div>
        </article>
      {/each}
    {:else}
      <div class="panel-body">{$_("tables.emptyMappings")}</div>
    {/if}
  </div>
</div>
