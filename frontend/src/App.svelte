<script lang="ts">
  // Routes between the sign-in flow and the signed-in views (SPEC §8).
  import { onDestroy, onMount } from 'svelte'
  import { fade, fly } from 'svelte/transition'
  import { Api, Events, type AuthState } from './lib/api'
  import { motion } from './lib/motion'
  import { setTheme } from './lib/theme'
  import Inbox from './views/Inbox.svelte'
  import Login from './views/Login.svelte'
  import OrgPicker from './views/OrgPicker.svelte'
  import Settings from './views/Settings.svelte'

  let auth: AuthState | undefined = $state()
  let view: 'inbox' | 'settings' = $state('inbox')

  const offs: (() => void)[] = []
  onMount(async () => {
    offs.push(Events.On('auth:changed', (ev) => (auth = ev.data)))
    offs.push(Events.On('nav', (ev) => (view = ev.data === 'settings' ? 'settings' : 'inbox')))
    auth = await Api.Auth()
    Api.Settings().then((s) => setTheme(s.theme)).catch(() => {})
  })
  onDestroy(() => offs.forEach((off) => off()))
</script>

{#if !auth}
  <div class="boot" aria-busy="true"><span class="spinner"></span></div>
{:else if auth.state === 'signed_in'}
  {#if view === 'settings'}
    <div class="view" in:fly={{ ...motion(180), x: 24 }}><Settings {auth} onback={() => (view = 'inbox')} /></div>
  {:else}
    <div class="view" in:fade={motion(140)}><Inbox onsettings={() => (view = 'settings')} /></div>
  {/if}
{:else if auth.state === 'choose_orgs'}
  <OrgPicker />
{:else}
  <Login {auth} />
{/if}

<style>
  .view {
    height: 100%;
  }
  .boot {
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }
</style>
