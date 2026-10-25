<script lang="ts">
  // 30×18 switch (prism-ui skill §Toggle).
  let {
    checked = $bindable(false),
    label,
    disabled = false,
    onchange,
  }: { checked?: boolean; label: string; disabled?: boolean; onchange?: (v: boolean) => void } = $props()

  function toggle() {
    if (disabled) return
    checked = !checked
    onchange?.(checked)
  }
</script>

<button type="button" class="toggle" role="switch" aria-checked={checked} aria-label={label} {disabled} onclick={toggle}>
  <span class="knob"></span>
</button>

<style>
  .toggle {
    position: relative;
    width: 30px;
    height: 18px;
    padding: 0;
    border: none;
    border-radius: 9px;
    background: var(--border-strong);
    cursor: pointer;
    flex: none;
  }
  .toggle[aria-checked='true'] {
    background: var(--accent);
  }
  .toggle:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: var(--bg);
    transition: transform 0.12s ease;
  }
  .toggle[aria-checked='true'] .knob {
    transform: translateX(12px);
  }
</style>
