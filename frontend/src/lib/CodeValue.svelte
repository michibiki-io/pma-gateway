<script>
  import { onDestroy } from "svelte";
  import { _ } from "./i18n/index.js";
  import { copyText, toDisplayString } from "./text.js";
  import TruncatedText from "./TruncatedText.svelte";

  export let label = "";
  export let value = "";
  export let fallback = "—";
  export let middle = true;
  export let lines = 1;
  export let copyable = false;

  let copied = false;
  let copiedResetTimer = null;

  $: normalizedValue = toDisplayString(value);
  $: hasValue = Boolean(normalizedValue);
  $: copyLabel = copied ? $_("common.copied") : $_("common.copy");

  onDestroy(() => {
    if (copiedResetTimer) {
      window.clearTimeout(copiedResetTimer);
    }
  });

  async function handleCopy() {
    const success = await copyText(normalizedValue);
    if (!success) {
      return;
    }
    copied = true;
    if (copiedResetTimer) {
      window.clearTimeout(copiedResetTimer);
    }
    copiedResetTimer = window.setTimeout(() => {
      copied = false;
      copiedResetTimer = null;
    }, 1600);
  }
</script>

<div class="code-value">
  <code class="code-value-chip" title={normalizedValue || fallback}>
    <TruncatedText
      value={normalizedValue}
      {fallback}
      {lines}
      {middle}
      monospace={true}
    />
  </code>
  {#if copyable && hasValue}
    <button
      type="button"
      class="icon-button code-value-copy"
      aria-label={$_("common.copyValueAria", { values: { label } })}
      title={copyLabel}
      on:click|stopPropagation={handleCopy}
    >
      <i
        class={copied ? "fa-solid fa-check" : "fa-regular fa-copy"}
        aria-hidden="true"
      ></i>
    </button>
  {/if}
</div>
