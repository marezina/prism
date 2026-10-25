<script lang="ts">
  // Avatar circle with an initials fallback on a pastel pair chosen by
  // hash(login), so a person always gets the same color (prism-ui skill).
  let { login = '', src = '', size = 28 }: { login?: string; src?: string; size?: number } = $props()
  let failed = $state(false)

  const palette = $derived.by(() => {
    let h = 0
    for (const ch of login) h = (h * 31 + ch.charCodeAt(0)) >>> 0
    return (h % 5) + 1
  })
  const initials = $derived(login.replace(/\[bot\]$/, '').slice(0, 2).toUpperCase() || '?')
</script>

{#if src && !failed}
  <img class="avatar" {src} alt="" width={size} height={size} style:width="{size}px" style:height="{size}px"
       onerror={() => (failed = true)} />
{:else}
  <span class="avatar initials" style:width="{size}px" style:height="{size}px"
        style:background="var(--av-{palette}-bg)" style:color="var(--av-{palette}-fg)" aria-hidden="true">
    {initials}
  </span>
{/if}

<style>
  .avatar {
    border-radius: 50%;
    flex: none;
    object-fit: cover;
  }
  .initials {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    line-height: 16px;
    font-weight: 600;
  }
</style>
