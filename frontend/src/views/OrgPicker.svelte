<script lang="ts">
  // Organization picker (SPEC §8 step 3); also used inside Settings.
  import { onMount } from 'svelte'
  import { Api, errorText, type Org } from '../lib/api'
  import Avatar from '../lib/Avatar.svelte'
  import Toggle from '../lib/Toggle.svelte'

  let { embedded = false }: { embedded?: boolean } = $props()

  let orgs: Org[] = $state([])
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let saved = $state(false)

  const anySelected = $derived(orgs.some((o) => o.selected))

  async function load() {
    loading = true
    error = ''
    try {
      orgs = (await Api.Orgs()) ?? []
    } catch (e) {
      error = errorText(e)
    } finally {
      loading = false
    }
  }
  onMount(load)

  async function save() {
    busy = true
    error = ''
    try {
      const owners = orgs.filter((o) => o.selected && !o.personal).map((o) => o.login)
      const personal = orgs.some((o) => o.personal && o.selected)
      await Api.ChooseOrgs(owners, personal)
      saved = true
      setTimeout(() => (saved = false), 1500)
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<section class:page={!embedded} class="picker">
  {#if !embedded}
    <header>
      <h1>What should PRism watch?</h1>
      <p class="muted">Pick the organizations whose pull requests you want notifications for. You can change this later in Settings.</p>
    </header>
  {/if}

  {#if loading}
    <div class="loading"><span class="spinner" aria-hidden="true"></span> Loading organizations…</div>
  {:else}
    <ul class="list">
      {#each orgs as org (org.login + org.personal)}
        <li class="row">
          <Avatar login={org.login} src={org.avatarUrl} size={24} />
          <div class="name">
            <span class="title">{org.personal ? 'Your personal repositories' : org.login}</span>
            {#if org.personal}<span class="meta">{org.login}</span>{/if}
            {#if org.ssoRequired}
              <span class="meta warn">SAML SSO not authorized ·
                <button class="link" onclick={() => Api.OpenURL(org.ssoUrl ?? '')}>Authorize</button> ·
                <button class="link" onclick={load}>Refresh</button>
              </span>
            {/if}
          </div>
          <Toggle label="Watch {org.personal ? 'personal repositories' : org.login}" bind:checked={org.selected}
                  disabled={org.ssoRequired} onchange={embedded ? save : undefined} />
        </li>
      {/each}
    </ul>
  {/if}

  {#if error}<p class="notice notice-error" role="alert">{error}</p>{/if}

  {#if !embedded}
    <footer>
      <button class="btn btn-primary" onclick={save} disabled={busy || loading || !anySelected}>
        {#if busy}<span class="spinner" aria-hidden="true"></span>{/if} Start watching
      </button>
    </footer>
  {:else if saved}
    <p class="muted saved" aria-live="polite">Saved</p>
  {/if}
</section>

<style>
  .picker {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .page {
    height: 100%;
    padding: var(--space-6);
  }
  header h1 {
    margin: 0 0 var(--space-2);
    font-size: 18px;
    line-height: 24px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  header p {
    margin: 0;
    font-size: 13px;
    line-height: 19px;
  }
  .loading {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: 13px;
    line-height: 19px;
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow-y: auto;
    flex: 1;
  }
  .page .list {
    border-top: 1px solid var(--border);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) 0;
    border-bottom: 1px solid var(--border);
  }
  .name {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .title {
    font-weight: 500;
  }
  .meta {
    font-size: 12px;
    line-height: 16px;
    color: var(--text-3);
  }
  .warn {
    color: var(--orange-fg);
  }
  footer .btn {
    width: 100%;
  }
  .saved {
    margin: 0;
    font-size: 12px;
    line-height: 16px;
  }
</style>
