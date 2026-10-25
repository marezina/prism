package detect

import "prism/internal/model"

// detectMention: a new comment, thread comment or review body in a watched PR
// that @mentions me or one of my teams (SPEC §3.2 mention). It has the lowest
// precedence, so anything classified higher for the same source wins.
func detectMention(d *diff) []candidate {
	var out []candidate
	add := func(key, body string, author model.Actor, url string, at model.Comment) {
		summary := d.mentionSummary(body)
		if summary == "" {
			return
		}
		out = append(out, candidate{source: key, event: d.event(model.Mention, key, author, summary, Excerpt(body), url, at.CreatedAt)})
	}
	for _, cm := range d.newComments() {
		add(commentKey(cm.DatabaseID), cm.Body, cm.Author, cm.URL, cm)
	}
	for _, tc := range d.newThreadComments() {
		add(commentKey(tc.DatabaseID), tc.Body, tc.Author, tc.URL, tc.Comment)
	}
	for _, r := range d.newReviews() {
		add(reviewKey(r), r.Body, r.Author, r.URL, model.Comment{CreatedAt: r.SubmittedAt})
	}
	return out
}

// mentionSummary is "mentioned you", "mentioned <org/team>", or "" when body
// mentions neither me nor my teams.
func (d *diff) mentionSummary(body string) string {
	if mentions(body, d.c.Me) {
		return "mentioned you"
	}
	for _, team := range d.c.MyTeams {
		if mentions(body, team) {
			return "mentioned " + team
		}
	}
	return ""
}
