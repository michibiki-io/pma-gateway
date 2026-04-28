<script>
  import CodeValue from "./CodeValue.svelte";
  import TruncatedText from "./TruncatedText.svelte";

  export let items = [];
  export let dense = false;
</script>

<dl class:key-value-list--dense={dense} class="key-value-list">
  {#each items as item, index (`${item.label}-${index}`)}
    <div class="key-value-list-row">
      <dt>{item.label}</dt>
      <dd>
        {#if item.kind === "code"}
          <CodeValue
            label={item.label}
            value={item.value}
            fallback={item.fallback ?? "—"}
            middle={item.middle ?? true}
            lines={item.lines ?? 1}
            copyable={item.copyable ?? false}
          />
        {:else}
          <TruncatedText
            value={item.value}
            fallback={item.fallback ?? "—"}
            lines={item.lines ?? 1}
            middle={item.middle ?? false}
            monospace={item.monospace ?? false}
          />
        {/if}
      </dd>
    </div>
  {/each}
</dl>
