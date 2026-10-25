<script lang="ts" module>
  // UI icons come straight from assets/ (24 px, currentColor, stroke 2).
  const files = import.meta.glob(['../../../assets/icons/ui/*.svg', '../../../assets/icons/app/prism-glyph.svg'], {
    query: '?raw',
    import: 'default',
    eager: true,
  }) as Record<string, string>

  const icons: Record<string, string> = {}
  for (const [path, svg] of Object.entries(files)) {
    icons[path.split('/').pop()!.replace('.svg', '')] = svg
  }
</script>

<script lang="ts">
  let { name, size = 16, label = '' }: { name: string; size?: number; label?: string } = $props()
</script>

<span
  class="icon"
  style:width="{size}px"
  style:height="{size}px"
  role={label ? 'img' : undefined}
  aria-label={label || undefined}
  aria-hidden={label ? undefined : 'true'}
>
  {@html icons[name] ?? ''}
</span>

<style>
  .icon {
    display: inline-flex;
    flex: none;
  }
  .icon :global(svg) {
    width: 100%;
    height: 100%;
  }
</style>
