import { mount } from 'svelte'
import '@fontsource-variable/inter'
import '@fontsource/jetbrains-mono/600.css'
import './app.css'
import './lib/theme'
import App from './App.svelte'

mount(App, { target: document.getElementById('app')! })
