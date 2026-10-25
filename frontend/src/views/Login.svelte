<script lang="ts">
  // Sign-in screens (SPEC §8): welcome → device code (or classic PAT).
  import { onDestroy } from 'svelte'
  import { Api, errorText, type AuthState } from '../lib/api'
  import Icon from '../lib/Icon.svelte'
  import { countdown, tick } from '../lib/time'

  let { auth }: { auth: AuthState } = $props()

  let mode: 'welcome' | 'pat' = $state('welcome')
  let token = $state('')
  let busy = $state(false)
  let error = $state('')
  let copied = $state(false)
  let now = $state(Date.now())
  const stop = tick(1000, (t) => (now = t))
  onDestroy(stop)

  const message = $derived(error || auth.message || '')

  async function signIn() {
    busy = true
    error = ''
    try {
      await Api.StartLogin()
    } catch (e) {
      error = errorText(e)
      if (error.includes('personal access token')) mode = 'pat'
    } finally {
      busy = false
    }
  }

  async function signInWithToken(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await Api.LoginWithToken(token)
      token = ''
    } catch (err) {
      error = errorText(err)
    } finally {
      busy = false
    }
  }

  async function copy() {
    await navigator.clipboard.writeText(auth.userCode ?? '')
    copied = true
    setTimeout(() => (copied = false), 1500)
  }
</script>

<main class="login">
  <div class="brand">
    <span class="glyph"><Icon name="prism-glyph" size={40} /></span>
    <h1>PRism</h1>
    <p class="muted">Pull-request activity on your desktop.</p>
  </div>

  {#if auth.state === 'awaiting_user'}
    <section class="device" aria-live="polite">
      <p>Enter this code on GitHub:</p>
      <div class="code-row">
        <code class="code">{auth.userCode}</code>
        <button class="icon-btn" aria-label="Copy code" onclick={copy}><Icon name={copied ? 'check' : 'copy'} /></button>
      </div>
      <div class="waiting">
        <span class="spinner" aria-hidden="true"></span>
        <span>Waiting for authorization… <span class="muted">expires in {countdown(auth.expiresAt, now)}</span></span>
      </div>
      <div class="actions">
        <button class="btn btn-secondary" onclick={() => Api.OpenURL(auth.verificationUri ?? '')}>
          <Icon name="external-link" /> Open GitHub again
        </button>
        <button class="btn btn-ghost" onclick={() => Api.CancelLogin()}>Cancel</button>
      </div>
    </section>
  {:else if mode === 'pat'}
    <form class="pat" onsubmit={signInWithToken}>
      <label for="token">Personal access token (classic)</label>
      <input id="token" class="field" type="password" autocomplete="off" spellcheck="false" placeholder="ghp_…"
             bind:value={token} />
      <p class="muted hint">Needs the scopes <code>repo</code>, <code>notifications</code> and <code>read:org</code>.
        Fine-grained tokens can't read notifications.</p>
      {#if message}<p class="notice notice-error" role="alert">{message}</p>{/if}
      <button class="btn btn-primary" type="submit" disabled={busy || !token.trim()}>
        {#if busy}<span class="spinner" aria-hidden="true"></span>{/if} Sign in
      </button>
      <button class="btn btn-ghost" type="button" onclick={() => { mode = 'welcome'; error = '' }}>Back</button>
    </form>
  {:else}
    <section class="welcome">
      {#if message}<p class="notice notice-error" role="alert">{message}</p>{/if}
      <button class="btn btn-primary" onclick={signIn} disabled={busy}>
        {#if busy}<span class="spinner" aria-hidden="true"></span>{/if} Sign in with GitHub
      </button>
      <button class="btn btn-ghost" onclick={() => { mode = 'pat'; error = '' }}>Use a personal access token instead</button>
      <p class="muted hint">PRism only reads from GitHub. Everything stays on this computer.</p>
    </section>
  {/if}
</main>

<style>
  .login {
    height: 100%;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: var(--space-8);
    padding: var(--space-8) var(--space-6);
  }
  .brand {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    text-align: center;
  }
  .glyph {
    color: var(--accent);
  }
  h1 {
    margin: 0;
    font-size: 18px;
    line-height: 24px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  p {
    margin: 0;
  }
  .welcome,
  .pat,
  .device {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .hint {
    font-size: 12px;
    line-height: 16px;
    text-align: center;
  }
  .pat .hint {
    text-align: left;
  }
  label {
    font-size: 13px;
    line-height: 19px;
    font-weight: 500;
  }
  .device {
    align-items: center;
    text-align: center;
  }
  .code-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .code {
    font-family: 'JetBrains Mono', var(--font-mono);
    font-size: 20px;
    line-height: 28px;
    font-weight: 600;
    letter-spacing: 0.08em;
    padding: var(--space-2) var(--space-4);
    background: var(--surface-2);
    border-radius: var(--radius);
  }
  .waiting {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: 13px;
    line-height: 19px;
  }
  .actions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: 100%;
  }
</style>
