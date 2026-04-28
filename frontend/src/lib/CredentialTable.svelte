<script>
  import { _ } from "./i18n/index.js";
  import CodeValue from "./CodeValue.svelte";
  import KeyValueList from "./KeyValueList.svelte";
  import TruncatedText from "./TruncatedText.svelte";

  export let credentials = [];
  export let onEdit = () => {};
  export let onToggleEnabled = () => {};
  export let onDelete = () => {};
  export let disabled = false;

  function mobileItems(item) {
    return [
      {
        label: $_("tables.headers.database"),
        value: `${item.dbHost}:${item.dbPort}`,
        kind: "code",
        copyable: true,
      },
      {
        label: $_("tables.headers.user"),
        value: item.dbUser,
        kind: "code",
        copyable: true,
      },
    ];
  }
</script>

<div class="panel">
  <div class="panel-header">
    <h2 class="font-semibold">{$_("tables.credentialsTitle")}</h2>
  </div>

  <div class="table-wrap desktop-data-table">
    <table class="table-middle">
      <thead>
        <tr>
          <th>{$_("tables.headers.name")}</th>
          <th>{$_("tables.headers.database")}</th>
          <th>{$_("tables.headers.user")}</th>
          <th>{$_("tables.headers.status")}</th>
          <th><span class="sr-only">{$_("tables.headers.actions")}</span></th>
        </tr>
      </thead>
      <tbody>
        {#each credentials as item}
          <tr>
            <td>
              <div class="list-primary-cell">
                <div class="font-medium">{item.name}</div>
                <div class="text-sm text-slate-500">
                  <TruncatedText value={item.id} monospace={true} lines={1} />
                </div>
              </div>
            </td>
            <td>
              <CodeValue
                label={$_("tables.headers.database")}
                value={`${item.dbHost}:${item.dbPort}`}
                copyable={false}
              />
            </td>
            <td>
              <CodeValue
                label={$_("tables.headers.user")}
                value={item.dbUser}
                copyable={false}
              />
            </td>
            <td>
              <label
                class:toggle-field--disabled={disabled}
                class="toggle-field"
              >
                <input
                  class="toggle-input"
                  type="checkbox"
                  checked={item.enabled}
                  {disabled}
                  aria-label={$_("tables.toggleStatusAria", {
                    values: { name: item.name },
                  })}
                  on:change={() => onToggleEnabled(item)}
                />
                <span class="toggle-control" aria-hidden="true">
                  <span class="toggle-thumb"></span>
                </span>
                <span class="toggle-state">
                  {item.enabled
                    ? $_("common.status.enabled")
                    : $_("common.status.disabled")}
                </span>
              </label>
            </td>
            <td>
              <div class="row-actions row-actions--inline">
                <button
                  type="button"
                  class="button secondary"
                  {disabled}
                  on:click={() => onEdit(item)}
                >
                  {$_("common.edit")}
                </button>
                <button
                  type="button"
                  class="button danger"
                  {disabled}
                  on:click={() => onDelete(item)}
                >
                  {$_("common.delete")}
                </button>
              </div>
            </td>
          </tr>
        {:else}
          <tr><td colspan="5">{$_("tables.emptyCredentials")}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="mobile-data-list">
    {#if credentials.length}
      {#each credentials as item}
        <article class="data-card">
          <div class="data-card-header">
            <div class="data-card-title-group">
              <div class="data-card-title">
                <TruncatedText value={item.name} lines={2} />
              </div>
              <div class="data-card-subtitle">
                <TruncatedText value={item.id} monospace={true} lines={1} />
              </div>
            </div>
          </div>

          <div class="data-card-body">
            <KeyValueList items={mobileItems(item)} dense={true} />

            <div class="key-value-list key-value-list--dense">
              <div class="key-value-list-row key-value-list-row--action">
                <dt>{$_("tables.headers.status")}</dt>
                <dd>
                  <label
                    class:toggle-field--disabled={disabled}
                    class="toggle-field"
                  >
                    <input
                      class="toggle-input"
                      type="checkbox"
                      checked={item.enabled}
                      {disabled}
                      aria-label={$_("tables.toggleStatusAria", {
                        values: { name: item.name },
                      })}
                      on:change={() => onToggleEnabled(item)}
                    />
                    <span class="toggle-control" aria-hidden="true">
                      <span class="toggle-thumb"></span>
                    </span>
                    <span class="toggle-state">
                      {item.enabled
                        ? $_("common.status.enabled")
                        : $_("common.status.disabled")}
                    </span>
                  </label>
                </dd>
              </div>
            </div>
          </div>

          <div class="row-actions row-actions--mobile">
            <button
              type="button"
              class="button secondary"
              {disabled}
              on:click={() => onEdit(item)}
            >
              {$_("common.edit")}
            </button>
            <button
              type="button"
              class="button danger"
              {disabled}
              on:click={() => onDelete(item)}
            >
              {$_("common.delete")}
            </button>
          </div>
        </article>
      {/each}
    {:else}
      <div class="panel-body">{$_("tables.emptyCredentials")}</div>
    {/if}
  </div>
</div>
