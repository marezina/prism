package app

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"prism/internal/model"
	"prism/internal/store"
)

func TestPRStatus(t *testing.T) {
	review := func(who string, s model.ReviewState) model.Review {
		return model.Review{Author: model.Actor{Login: who}, State: s}
	}
	tests := []struct {
		name string
		snap *model.PRSnapshot
		want *PRStatus
	}{
		{"no snapshot", nil, nil},
		{"empty", &model.PRSnapshot{}, &PRStatus{}},
		{"draft with conflict", &model.PRSnapshot{IsDraft: true, Mergeable: "CONFLICTING"},
			&PRStatus{Draft: true, Conflict: true}},
		{"latest review per reviewer counts", &model.PRSnapshot{Reviews: []model.Review{
			review("alice", model.ReviewChangesRequested),
			review("Alice", model.ReviewApproved),
			review("alice", model.ReviewCommented),
			review("bob", model.ReviewApproved),
			review("carol", model.ReviewChangesRequested),
			review("dave", model.ReviewPending),
		}}, &PRStatus{Approvals: 2, ChangesRequested: 1}},
		{"dismissal clears", &model.PRSnapshot{Reviews: []model.Review{
			review("alice", model.ReviewApproved),
			review("alice", model.ReviewDismissed),
		}}, &PRStatus{}},
		{"ci passing", &model.PRSnapshot{CI: &model.CI{State: model.CheckSuccess}}, &PRStatus{CI: CIPassing}},
		{"ci error is failing", &model.PRSnapshot{CI: &model.CI{State: model.CheckError}}, &PRStatus{CI: CIFailing}},
		{"ci expected is pending", &model.PRSnapshot{CI: &model.CI{State: model.CheckExpected}}, &PRStatus{CI: CIPending}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, prStatus(tt.snap)); diff != "" {
				t.Errorf("prStatus (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMuteAndMutedPRs(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	c := New(Options{Store: st})
	for _, s := range []store.PRState{
		{PRID: "PR_2", Repo: "acme/web", Number: 3, Role: store.RoleOther, Snapshot: &model.PRSnapshot{Title: "Web"}},
		{PRID: "PR_1", Repo: "acme/api", Number: 9, Role: store.RoleMine, Snapshot: &model.PRSnapshot{Title: "API"}},
		{PRID: "PR_3", Repo: "acme/api", Number: 1, Role: store.RoleMine, Snapshot: &model.PRSnapshot{Title: "Loud"}},
	} {
		if err := st.PutPRState(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"PR_2", "PR_1"} {
		if err := c.MutePR(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.MutePR(ctx, "PR_unknown"); err == nil {
		t.Error("muting an unknown PR succeeded")
	}
	got, err := c.MutedPRs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []MutedPR{
		{PRID: "PR_1", Repo: "acme/api", Number: 9, Title: "API", URL: "https://github.com/acme/api/pull/9"},
		{PRID: "PR_2", Repo: "acme/web", Number: 3, Title: "Web", URL: "https://github.com/acme/web/pull/3"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("MutedPRs (-want +got):\n%s", diff)
	}
	if err := c.UnmutePR(ctx, "PR_1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.MutedPRs(ctx); len(got) != 1 || got[0].PRID != "PR_2" {
		t.Errorf("after unmute: %+v", got)
	}
}
