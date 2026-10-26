import { cubicOut } from 'svelte/easing'

// Svelte transition params: short and eased, instant when the OS asks for
// reduced motion.
const reduced = typeof matchMedia === 'function' ? matchMedia('(prefers-reduced-motion: reduce)') : undefined

export function motion(duration: number) {
  return { duration: reduced?.matches ? 0 : duration, easing: cubicOut }
}
