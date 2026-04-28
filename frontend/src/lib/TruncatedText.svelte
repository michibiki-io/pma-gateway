<script>
  import { truncateMiddle } from "./text.js";

  export let value = "";
  export let title = "";
  export let fallback = "";
  export let lines = 1;
  export let middle = false;
  export let monospace = false;
  export let className = "";

  $: normalizedValue =
    value === null || value === undefined ? "" : String(value);
  $: resolvedValue = normalizedValue || fallback;
  $: displayValue =
    normalizedValue && middle ? truncateMiddle(normalizedValue) : resolvedValue;
  $: tooltip = title || normalizedValue || fallback;
</script>

<span
  class:truncate-text--monospace={monospace}
  class:truncate-text--multiline={lines > 1}
  class="truncate-text {className}"
  style:--truncate-lines={String(Math.max(1, lines))}
  title={tooltip}
>
  {displayValue}
</span>
