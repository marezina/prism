<script lang="ts">
  // Settings (SPEC §7): Account, Organizations, Events, Sound, Behavior,
  // Appearance, Advanced. Every change is saved right away.
  import { onDestroy, onMount } from 'svelte'
  import { Api, errorText, type AuthState, type Flags, type Settings } from '../lib/api'
  import Avatar from '../lib/Avatar.svelte'
  import Icon from '../lib/Icon.svelte'
  import Toggle from '../lib/Toggle.svelte'
  import { eventGroups } from '../lib/events'
  import { setTheme } from '../lib/theme'
  import OrgPicker from './OrgPicker.svelte'

  let { auth, onback }: { auth: AuthState; onback: () => void } = $props()

  let st: Settings | undefined = $state()
  let sounds: string[] = $state([])
  let version = $state('')
  let newer = $state({ version: '', url: '' })
  let error = $state('')
  let saved = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let confirmReset = $state(false)

  async function reset() {
    if (!confirmReset) {
      confirmReset = true
      setTimeout(() => (confirmReset = false), 4000)
      return
    }
    try {
      await Api.ResetLocalData()
    } catch (e) {
      error = errorText(e)
    }
  }

  onMount(async () => {
    try {
      ;[st, sounds, version] = await Promise.all([Api.Settings(), Api.SoundNames().then((s) => s ?? []), Api.Version()])
      newer = await Api.Update()
    } catch (e) {
      error = errorText(e)
    }
  })
  onDestroy(() => clearTimeout(timer))

  function events(): Record<string, Flags> {
    return (st!.events ?? {}) as Record<string, Flags>
  }

  // Debounced save of the whole settings object.
  function save() {
    if (!st) return
    setTheme(st.theme)
    clearTimeout(timer)
    timer = setTimeout(async () => {
      try {
        await Api.SaveSettings($state.snapshot(st!) as Settings)
        error = ''
        saved = true
        setTimeout(() => (saved = false), 1200)
      } catch (e) {
        error = errorText(e)
      }
    }, 300)
  }

  const intervals = [
    { value: 60, label: 'Every minute' },
    { value: 120, label: 'Every 2 minutes' },
    { value: 300, label: 'Every 5 minutes' },
  ]
  const themes = [
    { value: 'system', label: 'System' },
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
  ]
  const soundLabel = (s: string) => s.replace('-', ' ').replace(/^./, (c) => c.toUpperCase())
</script>

<div class="settings">
  <header class="top">
    <button class="icon-btn" aria-label="Back to notifications" onclick={onback}><Icon name="chevron-left" /></button>
    <h1>Settings</h1>
    <span class="status muted" aria-live="polite">{saved ? 'Saved' : ''}</span>
  </header>

  {#if error}<p class="notice notice-error err" role="alert">{error}</p>{/if}

  {#if st}
    <div class="scroll">
      <section>
        <h2>Account</h2>
        <div class="account">
          <Avatar login={auth.login} src={auth.avatarUrl} size={32} />
          <div class="grow">
            <div class="strong">{auth.login}</div>
            <div class="meta">Signed in with GitHub · read-only access</div>
          </div>
          <button class="btn btn-secondary btn-small" onclick={() => Api.Logout()}>Sign out</button>
        </div>
      </section>

      <section>
        <h2>Organizations</h2>
        <OrgPicker embedded />
      </section>

      <section>
        <h2>Events</h2>
        <div class="events" role="table" aria-label="Event delivery">
          <div class="ev-row ev-head" role="row">
            <span role="columnheader">Event</span>
            <span role="columnheader">List</span>
            <span role="columnheader">Toast</span>
            <span role="columnheader">Sound</span>
          </div>
          {#each eventGroups as g (g.title)}
            <div class="ev-row ev-group" role="row"><span role="rowheader">{g.title}</span></div>
            {#each g.events as e (e.type)}
              {@const f = events()[e.type] ?? { list: false, toast: false, sound: false }}
              <div class="ev-row" role="row">
                <span class="ev-name" role="cell">
                  <span>{e.label}</span>
                  <span class="meta">{e.hint}</span>
                </span>
                <span role="cell"><Toggle label="{e.label}: show in list" checked={f.list}
                  onchange={(v) => { events()[e.type] = { ...f, list: v }; save() }} /></span>
                <span role="cell"><Toggle label="{e.label}: toast" checked={f.toast}
                  onchange={(v) => { events()[e.type] = { ...f, toast: v }; save() }} /></span>
                <span role="cell"><Toggle label="{e.label}: sound" checked={f.sound}
                  onchange={(v) => { events()[e.type] = { ...f, sound: v }; save() }} /></span>
              </div>
            {/each}
          {/each}
        </div>
      </section>

      <section>
        <h2>Sound</h2>
        <div class="line">
          <span class="grow">Play sounds</span>
          <Toggle label="Play sounds" bind:checked={st.sound.enabled} onchange={save} />
        </div>
        <div class="line">
          <label class="grow" for="volume">Volume</label>
          <input id="volume" type="range" min="0" max="1" step="0.05" bind:value={st.sound.volume} onchange={save}
                 disabled={!st.sound.enabled} />
        </div>
        <div class="line">
          <label class="grow" for="people-sound">People events</label>
          <select id="people-sound" class="field" bind:value={st.sound.name} onchange={save}>
            {#each sounds as s}<option value={s}>{soundLabel(s)}</option>{/each}
          </select>
          <button class="icon-btn" aria-label="Play people sound" onclick={() => Api.PreviewSound(st!.sound.name, st!.sound.volume)}>
            <Icon name="refresh" size={14} />
          </button>
        </div>
        <div class="line">
          <label class="grow" for="alert-sound">CI failures and conflicts</label>
          <select id="alert-sound" class="field" bind:value={st.sound.alertName} onchange={save}>
            {#each sounds as s}<option value={s}>{soundLabel(s)}</option>{/each}
          </select>
          <button class="icon-btn" aria-label="Play alert sound" onclick={() => Api.PreviewSound(st!.sound.alertName, st!.sound.volume)}>
            <Icon name="refresh" size={14} />
          </button>
        </div>
      </section>

      <section>
        <h2>Behavior</h2>
        <div class="line">
          <label class="grow" for="interval">Check GitHub</label>
          <select id="interval" class="field" bind:value={st.pollIntervalSec} onchange={save}>
            {#each intervals as i}<option value={i.value}>{i.label}</option>{/each}
          </select>
        </div>
        <div class="line"><span class="grow">Ignore bots</span>
          <Toggle label="Ignore bots" bind:checked={st.ignoreBots} onchange={save} /></div>
        <div class="line"><span class="grow">Watch draft PRs</span>
          <Toggle label="Watch draft PRs" bind:checked={st.includeDrafts} onchange={save} /></div>
        <div class="line">
          <span class="grow">Group bursts<span class="meta block">3 or more at once become one notification</span></span>
          <Toggle label="Group bursts" bind:checked={st.burstGrouping} onchange={save} />
        </div>
        <div class="line">
          <span class="grow">Quiet hours<span class="meta block">No toasts or sounds; items are still listed</span></span>
          <Toggle label="Quiet hours" bind:checked={st.quietHours.enabled} onchange={save} />
        </div>
        {#if st.quietHours.enabled}
          <div class="line indent">
            <label for="qh-from">From</label>
            <input id="qh-from" class="field" type="time" bind:value={st.quietHours.from} onchange={save} />
            <label for="qh-to">to</label>
            <input id="qh-to" class="field" type="time" bind:value={st.quietHours.to} onchange={save} />
          </div>
          <div class="line indent"><span class="grow">All weekend</span>
            <Toggle label="Quiet all weekend" bind:checked={st.quietHours.weekends} onchange={save} /></div>
        {/if}
        <div class="line"><span class="grow">Start PRism when I sign in</span>
          <Toggle label="Start PRism when I sign in" bind:checked={st.autostart} onchange={save} /></div>
      </section>

      <section>
        <h2>Appearance</h2>
        <div class="segmented" role="radiogroup" aria-label="Theme">
          {#each themes as t}
            <button role="radio" aria-checked={st.theme === t.value} class:on={st.theme === t.value}
                    onclick={() => { st!.theme = t.value; save() }}>{t.label}</button>
          {/each}
        </div>
      </section>

      <section>
        <h2>Advanced</h2>
        <div class="line"><span class="grow">Version</span><span class="muted">{version}</span></div>
        {#if newer.version}
          <p class="notice notice-info">
            PRism {newer.version} is available.
            <button class="link" onclick={() => Api.OpenURL(newer.url)}>Open the release page</button>
          </p>
        {/if}
        <div class="line">
          <span class="grow">Logs</span>
          <button class="btn btn-secondary btn-small" onclick={() => Api.OpenLogsFolder()}>Open logs folder</button>
        </div>
        <div class="line">
          <span class="grow">Reset local data<span class="meta block">Deletes notifications, settings and cached avatars on this computer. You stay signed in.</span></span>
          <button class="btn btn-small" class:btn-secondary={!confirmReset} class:danger={confirmReset} onclick={reset}>
            {confirmReset ? 'Click again to reset' : 'Reset…'}
          </button>
        </div>
      </section>
    </div>
  {/if}
</div>

<style>
  .settings {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .top {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-4) var(--space-4) var(--space-2);
    border-bottom: 1px solid var(--border);
  }
  h1 {
    margin: 0;
    flex: 1;
    font-size: 18px;
    line-height: 24px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .status {
    font-size: 12px;
    line-height: 16px;
  }
  .err {
    margin: var(--space-2) var(--space-4) 0;
  }
  .scroll {
    flex: 1;
    overflow-y: auto;
    padding: 0 var(--space-4) var(--space-8);
  }
  section {
    padding: var(--space-5) 0;
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  section:last-child {
    border-bottom: none;
  }
  h2 {
    margin: 0;
    font-size: 20px;
    line-height: 28px;
    font-weight: 600;
  }
  .account,
  .line {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .indent {
    padding-left: var(--space-4);
  }
  .grow {
    flex: 1;
    min-width: 0;
  }
  .strong {
    font-weight: 600;
  }
  .meta {
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
  }
  .block {
    display: block;
  }
  .events {
    display: flex;
    flex-direction: column;
  }
  .ev-row {
    display: grid;
    grid-template-columns: 1fr 44px 44px 44px;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--border);
  }
  .ev-head {
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
    font-weight: 500;
  }
  .ev-group {
    display: block;
    padding-top: var(--space-4);
    font-size: 12px;
    line-height: 16px;
    font-weight: 600;
    color: var(--text-2);
  }
  .ev-name {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  input[type='range'] {
    width: 160px;
    accent-color: var(--accent);
  }
  .danger {
    background: var(--red-bg);
    color: var(--red-fg);
  }
  .segmented {
    display: inline-flex;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    overflow: hidden;
    align-self: flex-start;
  }
  .segmented button {
    border: none;
    background: var(--bg);
    padding: var(--space-1) var(--space-4);
    font-size: 13px;
    line-height: 19px;
    cursor: pointer;
    color: var(--text-2);
  }
  .segmented button + button {
    border-left: 1px solid var(--border-strong);
  }
  .segmented button.on {
    background: var(--selected);
    color: var(--text);
    font-weight: 500;
  }
</style>
