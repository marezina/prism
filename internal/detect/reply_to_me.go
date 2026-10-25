package detect

import "prism/internal/model"

const replySummary = "replied to your comment"

// detectReplyToMe: on any watched PR (SPEC §3.1 comment.reply_to_me)
//
//	(a) a thread comment replying to my comment, or in a thread where I
//	    commented earlier;
//	(b) a conversation comment that quote-replies one of my comments, or
//	    @mentions me after I commented on the PR.
func detectReplyToMe(d *diff) []candidate {
	var out []candidate
	for _, tc := range d.newThreadComments() {
		if tc.isRoot() || !d.repliesToMeInThread(tc) {
			continue
		}
		out = append(out, candidate{
			source: commentKey(tc.DatabaseID),
			event: d.event(model.CommentReplyToMe, commentKey(tc.DatabaseID), tc.Author, replySummary,
				Excerpt(tc.Body), tc.URL, tc.CreatedAt),
		})
	}
	for _, cm := range d.newComments() {
		quoted := quotes(cm.Body, d.myBodies)
		mentioned := mentions(cm.Body, d.c.Me) && d.iCommentedBefore(cm.CreatedAt)
		if !quoted && !mentioned {
			continue
		}
		out = append(out, candidate{
			source: commentKey(cm.DatabaseID),
			event: d.event(model.CommentReplyToMe, commentKey(cm.DatabaseID), cm.Author, replySummary,
				Excerpt(cm.Body), cm.URL, cm.CreatedAt),
		})
	}
	return out
}

// repliesToMeInThread: I wrote a comment before tc in its thread. This covers
// replyTo-is-mine too, because the replied-to comment always comes earlier.
func (d *diff) repliesToMeInThread(tc threadComment) bool {
	for _, earlier := range tc.thread.Comments[:tc.index] {
		if d.isMe(earlier.Author) {
			return true
		}
	}
	return false
}
