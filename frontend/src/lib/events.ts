import type { Notification } from './api'

// Event → leading visual (prism-ui skill §Event visuals, assets/README.md).
export type Tint = 'green' | 'red' | 'orange' | 'grey' | 'purple' | 'blue'
export type Visual = { kind: 'avatar'; fallback: string } | { kind: 'icon'; icon: string; tint: Tint }

const systemVisuals: Record<string, { icon: string; tint: Tint }> = {
  'ci.passed': { icon: 'check-circle', tint: 'green' },
  'ci.failed': { icon: 'x-circle', tint: 'red' },
  'merge.conflict': { icon: 'conflict', tint: 'orange' },
  'commits.after_my_review': { icon: 'commit', tint: 'grey' },
  'my_pr.closed': { icon: 'merge', tint: 'purple' },
  'my_pr.opened': { icon: 'pull-request', tint: 'grey' },
}

// Icon used when a person event has no avatar.
const personIcons: Record<string, string> = {
  'my_pr.approved': 'check-circle',
  'my_pr.changes_requested': 'changes-requested',
  'my_pr.comment': 'comment',
  'my_pr.thread_reply': 'reply',
  'comment.reply_to_me': 'reply',
  'review.requested': 'review-requested',
  mention: 'mention',
}

export function visual(type: string): Visual {
  const sys = systemVisuals[type]
  if (sys) return { kind: 'icon', ...sys }
  return { kind: 'avatar', fallback: personIcons[type] ?? 'comment' }
}

/** Events whose excerpt box is shown (comments, replies, mentions, reviews with a body). */
export function showsExcerpt(type: string): boolean {
  return !systemVisuals[type] && type !== 'review.requested'
}

// Comment-like events: clicking the row opens the comment on GitHub. Every other
// row only marks itself read; the PR title above the group opens the PR.
const linkTypes = new Set(['my_pr.comment', 'my_pr.thread_reply', 'comment.reply_to_me', 'mention'])

export function opensLink(type: string): boolean {
  return linkTypes.has(type)
}

export const tabs = [
  { id: 'all', label: 'All' },
  { id: 'mine', label: 'My PRs' },
  { id: 'reviews', label: 'Reviews' },
  { id: 'mentions', label: 'Mentions' },
  { id: 'ci', label: 'CI' },
] as const
export type TabId = (typeof tabs)[number]['id']

const mentionTypes = new Set(['mention', 'comment.reply_to_me'])
const ciTypes = new Set(['ci.failed', 'ci.passed', 'merge.conflict'])

export function inTab(n: Notification, tab: TabId): boolean {
  switch (tab) {
    case 'all':
      return true
    case 'mine':
      return n.mine
    case 'reviews':
      return !n.mine && !mentionTypes.has(n.type)
    case 'mentions':
      return mentionTypes.has(n.type)
    case 'ci':
      return ciTypes.has(n.type)
  }
}

export type PRGroup = { prId: string; repo: string; number: number; title: string; url: string; mine: boolean; items: Notification[] }

/** Groups notifications by PR, keeping the order of each PR's newest item. */
export function groupByPR(list: Notification[]): PRGroup[] {
  const groups = new Map<string, PRGroup>()
  for (const n of list) {
    let g = groups.get(n.prId)
    if (!g) {
      g = { prId: n.prId, repo: n.repo, number: n.prNumber, title: n.prTitle, url: n.prUrl, mine: n.mine, items: [] }
      groups.set(n.prId, g)
    }
    g.items.push(n)
  }
  return [...groups.values()]
}

// Settings → Events table (SPEC §3): grouped by where the event happens,
// alphabetical within a group.
export type EventSetting = { type: string; label: string; hint: string }

export const eventGroups: { title: string; events: EventSetting[] }[] = [
  {
    title: 'Your PRs',
    events: [
      { type: 'my_pr.approved', label: 'Approved', hint: 'Someone approves your PR' },
      { type: 'my_pr.changes_requested', label: 'Changes requested', hint: 'Someone requests changes on your PR' },
      { type: 'my_pr.comment', label: 'Comments', hint: 'New comments and reviews on your PR' },
      { type: 'my_pr.closed', label: 'Merged or closed', hint: 'Your PR is merged or closed' },
      { type: 'my_pr.opened', label: 'Opened', hint: 'A PR you opened starts being watched' },
      { type: 'my_pr.thread_reply', label: 'Thread replies', hint: 'Replies in review threads on your PR' },
    ],
  },
  {
    title: 'Checks and merging',
    events: [
      { type: 'ci.failed', label: 'CI failed', hint: 'Checks fail on your PR' },
      { type: 'ci.passed', label: 'CI passed', hint: 'Checks turn green on your PR' },
      { type: 'merge.conflict', label: 'Merge conflicts', hint: 'Your PR can no longer be merged cleanly' },
    ],
  },
  {
    title: 'Other PRs',
    events: [
      { type: 'commits.after_my_review', label: 'Commits after your review', hint: 'A PR you reviewed gets new commits' },
      { type: 'mention', label: 'Mentions', hint: 'You or your team are @mentioned' },
      { type: 'comment.reply_to_me', label: 'Replies to you', hint: 'Someone replies to your comment, anywhere' },
      { type: 'review.requested', label: 'Review requests', hint: 'You or your team are asked to review' },
    ],
  },
]
