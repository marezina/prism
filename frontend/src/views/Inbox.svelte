<script lang="ts">
  // The popup's main view: unread notifications grouped by PR (prism-ui skill).
  import { onDestroy, onMount, tick as nextTick } from 'svelte'
  import { slide } from 'svelte/transition'
  import { Api, Events, errorText, type Notification, type SyncStatus } from '../lib/api'
  import Avatar from '../lib/Avatar.svelte'
  import Icon from '../lib/Icon.svelte'
  import { groupByPR, inTab, opensLink, showsExcerpt, statusChips, tabs, visual, type TabId } from '../lib/events'
  import { motion } from '../lib/motion'
  import { relative, tick } from '../lib/time'

  let { onsettings }: { onsettings: () => void } = $props()

  let items: Notification[] = $state([])
  let loaded = $state(false)
  let tab: TabId = $state('all')
  let status: SyncStatus = $state({ offline: false, syncing: false, watched: 0, rateRemaining: 0, skipped: [] })
  let error = $state('')
  let now = $state(Date.now())
  let listEl: HTMLElement | undefined = $state()

  const visible = $derived(items.filter((n) => inTab(n, tab)))
  const groups = $derived(groupByPR(visible))
  const counts = $derived(Object.fromEntries(tabs.map((t) => [t.id, items.filter((n) => inTab(n, t.id)).length])))

  async function load() {
    try {
      items = (await Api.Unread()) ?? []
      error = ''
    } catch (e) {
      error = errorText(e)
    } finally {
      loaded = true
    }
  }

  const offs: (() => void)[] = []
  onMount(() => {
    load()
    Api.Status().then((s) => (status = s))
    offs.push(Events.On('unread:changed', () => load()))
    offs.push(Events.On('sync:status', (ev) => (status = ev.data)))
    offs.push(tick(30_000, (t) => (now = t)))
  })
  onDestroy(() => offs.forEach((off) => off()))

  // Optimistic: remove first, restore and show an error if Go refuses.
  async function remove(ids: number[], call: () => Promise<void>) {
    const before = items
    items = items.filter((n) => !ids.includes(n.id))
    try {
      await call()
    } catch (e) {
      items = before
      error = errorText(e)
    }
  }

  const open = (n: Notification) => remove([n.id], () => Api.Open(n.id))
  const markRead = (n: Notification) => remove([n.id], () => Api.MarkRead([n.id]))
  const activate = (n: Notification) => (opensLink(n.type) ? open(n) : markRead(n))
  const markPR = (prId: string) =>
    remove(items.filter((n) => n.prId === prId).map((n) => n.id), () => Api.MarkPRRead(prId))
  const mutePR = (prId: string) =>
    remove(items.filter((n) => n.prId === prId).map((n) => n.id), () => Api.MutePR(prId))
  const markAll = () => remove(items.map((n) => n.id), () => Api.MarkAllRead())

  // Keyboard: ↑/↓ move between rows, Enter opens (native button), E marks read.
  function rows(): HTMLButtonElement[] {
    return listEl ? [...listEl.querySelectorAll<HTMLButtonElement>('button[data-row]')] : []
  }
  async function onkeydown(e: KeyboardEvent) {
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement) return
    const all = rows()
    const i = all.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const next = e.key === 'ArrowDown' ? Math.min(all.length - 1, i + 1) : Math.max(0, i - 1)
      all[next]?.focus()
    } else if ((e.key === 'e' || e.key === 'E') && i >= 0) {
      e.preventDefault()
      const n = visible.find((x) => x.id === Number(all[i].dataset.row))
      if (n) {
        await markRead(n)
        await nextTick()
        const after = rows()
        after[Math.min(i, after.length - 1)]?.focus()
      }
    }
  }

  function actionText(n: Notification): string {
    return n.summary
  }
</script>

<svelte:window {onkeydown} />

<div class="inbox">
  <header class="top">
    <h1>PRism</h1>
    <div class="tools">
      <button class="icon-btn" aria-label="Mark all as read" title="Mark all as read" onclick={markAll}
              disabled={items.length === 0}><Icon name="check" /></button>
      <button class="icon-btn" aria-label="Settings" title="Settings" onclick={onsettings}><Icon name="settings" /></button>
    </div>
  </header>

  {#if status.offline}
    <div class="banner" role="status">
      <Icon name="shield-alert" size={14} />
      {#if status.ssoUrl}
        <span>An organization needs SSO authorization.</span>
        <button class="link" onclick={() => Api.OpenURL(status.ssoUrl ?? '')}>Authorize</button>
      {:else}
        <span title={status.error}>Offline · retrying automatically</span>
      {/if}
    </div>
  {/if}

  {#each status.skipped ?? [] as org (org.login)}
    <div class="banner" role="status">
      <Icon name="shield-alert" size={14} />
      <span class="grow" title={org.error}>
        {org.ssoUrl ? `${org.login} needs SSO authorization` : `Can't read ${org.login} right now`}
      </span>
      {#if org.ssoUrl}<button class="link" onclick={() => Api.OpenURL(org.ssoUrl ?? '')}>Authorize</button>{/if}
    </div>
  {/each}

  <nav class="tabs" aria-label="Filter">
    {#each tabs as t}
      <button class="tab" class:active={tab === t.id} aria-pressed={tab === t.id} onclick={() => (tab = t.id)}>
        {t.label}{#if counts[t.id] > 0}<span class="count">{counts[t.id]}</span>{/if}
      </button>
    {/each}
  </nav>

  {#if error}<p class="notice notice-error err" role="alert">{error}</p>{/if}

  <!-- Keyed by tab: switching tabs swaps the list instantly; rows and groups
       animate only when they arrive or are read within a tab. -->
  {#key tab}
  <div class="list" bind:this={listEl}>
    {#if loaded && groups.length === 0}
      <div class="empty">
        <span class="glyph"><Icon name="prism-glyph" size={32} /></span>
        <p>You're all caught up</p>
        {#if tab !== 'all' && items.length > 0}<p class="muted">Nothing in this tab.</p>{/if}
      </div>
    {/if}

    {#each groups as g (g.prId)}
      {@const chips = statusChips(g.status)}
      <section class="group" aria-label="{g.repo} #{g.number}" transition:slide={motion(180)}>
        <div class="group-head">
          <div class="pr">
            <div class="repo-line">
              <span class="repo">{g.repo}</span>
              <span class="pill" class:review={!g.mine}>{g.mine ? 'Your PR' : 'Review'}</span>
            </div>
            <button class="pr-title" title="Open the PR" onclick={() => Api.OpenURL(g.url)}>
              <span class="num">#{g.number}</span> <span class="title">{g.title}</span>
            </button>
            {#if chips.length > 0}
              <ul class="chips" aria-label="PR status">
                {#each chips as c (c.label)}
                  <li class="chip fg-{c.tint ?? 'none'}">
                    {#if c.icon}<Icon name={c.icon} size={12} />{/if}{c.label}
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
          {#if g.status}
            <button class="icon-btn" aria-label="Mute PR" title="Mute this PR until it closes"
                    onclick={() => mutePR(g.prId)}><Icon name="bell-off" /></button>
          {/if}
          <button class="btn btn-secondary btn-small" onclick={() => markPR(g.prId)}>Mark PR read</button>
        </div>

        {#each g.items as n (n.id)}
          {@const v = visual(n.type)}
          {@const link = opensLink(n.type)}
          <button class="row" data-row={n.id} title={link ? 'Open comment' : 'Mark as read'} onclick={() => activate(n)}
                  transition:slide={motion(160)}>
            {#if v.kind === 'icon'}
              <span class="tile tint-{v.tint}"><Icon name={v.icon} /></span>
            {:else if n.actorAvatar || n.actorLogin}
              <Avatar login={n.actorLogin} src={n.actorAvatar} />
            {:else}
              <span class="tile tint-grey"><Icon name={v.fallback} /></span>
            {/if}
            <span class="body">
              <span class="text">
                {#if n.actorLogin && v.kind === 'avatar'}<strong>{n.actorLogin}</strong>{' '}{/if}{actionText(n)}
              </span>
              {#if n.excerpt && showsExcerpt(n.type)}<span class="excerpt">{n.excerpt}</span>{/if}
            </span>
            <span class="side">
              <span class="time">{relative(n.occurredAt, now)}</span>
              <span class="dot" aria-label="unread"></span>
            </span>
          </button>
        {/each}
      </section>
    {/each}
  </div>
  {/key}

  <footer class="foot">
    <span class="muted">
      {#if status.syncing}Syncing…{:else if status.lastSync}Synced {relative(status.lastSync, now) === 'now' ? 'just now' : relative(status.lastSync, now) + ' ago'}
        · {status.watched} PRs watched{:else}Syncing…{/if}
    </span>
    <button class="icon-btn" class:spinning={status.syncing} aria-label={status.syncing ? 'Syncing' : 'Refresh now'}
            title={status.syncing ? 'Syncing…' : 'Refresh now'} aria-busy={status.syncing} disabled={status.syncing}
            onclick={() => { status = { ...status, syncing: true }; Api.RefreshNow() }}>
      <Icon name="refresh" size={14} />
    </button>
  </footer>
</div>

<style>
  .inbox {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-4) var(--space-4) var(--space-2);
  }
  h1 {
    margin: 0;
    font-size: 18px;
    line-height: 24px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .tools {
    display: flex;
    gap: var(--space-1);
  }
  .icon-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .icon-btn.spinning:disabled {
    opacity: 1;
  }
  .spinning :global(svg) {
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spinning :global(svg) {
      animation-duration: 3s;
    }
  }
  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0 var(--space-4) var(--space-2);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius);
    background: var(--orange-bg);
    color: var(--orange-fg);
    font-size: 12px;
    line-height: 16px;
  }
  .grow {
    flex: 1;
    min-width: 0;
  }
  .tabs {
    display: flex;
    gap: var(--space-4);
    padding: 0 var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .tab {
    border: none;
    background: none;
    padding: var(--space-2) 0;
    margin-bottom: -1px;
    border-bottom: 2px solid transparent;
    color: var(--text-3);
    font-size: 13px;
    line-height: 19px;
    font-weight: 500;
    cursor: pointer;
    display: inline-flex;
    gap: var(--space-1);
    align-items: center;
  }
  .tab.active {
    color: var(--text);
    border-bottom-color: var(--text);
  }
  .count {
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
  }
  .err {
    margin: var(--space-2) var(--space-4) 0;
  }
  .list {
    flex: 1;
    overflow-y: auto;
  }
  .empty {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    color: var(--text-2);
  }
  .empty p {
    margin: 0;
  }
  .empty .glyph {
    color: var(--text-3);
  }
  .group {
    padding: var(--space-3) var(--space-2);
    border-bottom: 1px solid var(--border);
  }
  .group-head {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: 0 var(--space-2) var(--space-1);
  }
  .pr {
    flex: 1;
    min-width: 0;
  }
  .repo-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .repo {
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
  }
  .pill {
    font-size: 12px;
    line-height: 16px;
    font-weight: 500;
    padding: 0 var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--grey-bg);
    color: var(--grey-fg);
  }
  .pill.review {
    background: var(--blue-bg);
    color: var(--blue-fg);
  }
  .pr-title {
    display: block;
    width: 100%;
    border: none;
    background: none;
    padding: 0;
    text-align: left;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pr-title:hover .title {
    text-decoration: underline;
  }
  .num {
    color: var(--text-3);
    font-weight: 500;
  }
  .title {
    font-weight: 600;
  }
  .row {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-2);
    border: none;
    border-radius: var(--radius);
    background: none;
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    background: var(--hover);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    margin: var(--space-1) 0 0;
    padding: 0;
    list-style: none;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
  }
  .fg-green { color: var(--green-fg); }
  .fg-red { color: var(--red-fg); }
  .fg-orange { color: var(--orange-fg); }
  .tile {
    width: 28px;
    height: 28px;
    border-radius: var(--radius);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
  }
  .tint-green { background: var(--green-bg); color: var(--green-fg); }
  .tint-red { background: var(--red-bg); color: var(--red-fg); }
  .tint-orange { background: var(--orange-bg); color: var(--orange-fg); }
  .tint-grey { background: var(--grey-bg); color: var(--grey-fg); }
  .tint-purple { background: var(--purple-bg); color: var(--purple-fg); }
  .tint-blue { background: var(--blue-bg); color: var(--blue-fg); }
  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .text strong {
    font-weight: 600;
  }
  .excerpt {
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 13px;
    line-height: 19px;
  }
  .side {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-top: 2px;
  }
  .time {
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--accent-dot);
  }
  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--border);
    font-size: 12px;
    line-height: 16px;
  }
</style>
