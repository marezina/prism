package detect

import "prism/internal/model"

// detectThreadReply: new reply in a review thread on my PR (SPEC §3.1
// my_pr.thread_reply). Replies in threads where I commented earlier are also
// proposed here and lose to comment.reply_to_me in the precedence resolver.
func detectThreadReply(d *diff) []candidate {
	if !d.mine {
		return nil
	}
	var out []candidate
	for _, tc := range d.newThreadComments() {
		if tc.isRoot() {
			continue
		}
		out = append(out, candidate{
			source: commentKey(tc.DatabaseID),
			event: d.event(model.MyPRThreadReply, commentKey(tc.DatabaseID), tc.Author,
				"replied in a review thread on your PR", Excerpt(tc.Body), tc.URL, tc.CreatedAt),
		})
	}
	return out
}
