package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"prism/internal/gh"
)

// fakeOAuth serves the device code endpoint and replays poll responses.
type fakeOAuth struct {
	t        *testing.T
	polls    []map[string]any // one per poll; the last repeats
	nPolls   int
	lastForm url.Values
}

func (f *fakeOAuth) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.lastForm = r.PostForm
		if r.Header.Get("Accept") != "application/json" {
			f.t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		writeJSON(w, map[string]any{
			"device_code": "dev-123", "user_code": "ABCD-1234",
			"verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 5,
		})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.lastForm = r.PostForm
		resp := f.polls[min(f.nPolls, len(f.polls)-1)]
		f.nPolls++
		writeJSON(w, resp)
	})
	srv := httptest.NewServer(mux)
	f.t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// testFlow returns a flow whose clock only advances when it sleeps.
func testFlow(srv *httptest.Server) (*DeviceFlow, *[]time.Duration) {
	now := time.Unix(1_800_000_000, 0)
	var sleeps []time.Duration
	f := NewDeviceFlow("client-1", gh.Endpoints{Web: srv.URL}, "test")
	f.now = func() time.Time { return now }
	f.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		now = now.Add(d)
		return nil
	}
	return f, &sleeps
}

func TestDeviceStart(t *testing.T) {
	fake := &fakeOAuth{t: t}
	flow, _ := testFlow(fake.server())
	dc, err := flow.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "ABCD-1234" || dc.VerificationURI != "https://github.com/login/device" || dc.Interval != 5*time.Second {
		t.Errorf("device code = %+v", dc)
	}
	if !dc.ExpiresAt.Equal(time.Unix(1_800_000_900, 0)) {
		t.Errorf("ExpiresAt = %v", dc.ExpiresAt)
	}
	if got := fake.lastForm.Get("scope"); got != "repo notifications read:org" {
		t.Errorf("scope = %q", got)
	}
	if got := fake.lastForm.Get("client_id"); got != "client-1" {
		t.Errorf("client_id = %q", got)
	}
}

func TestDeviceStartNoClientID(t *testing.T) {
	flow := NewDeviceFlow("", gh.DotCom, "test")
	if _, err := flow.Start(context.Background()); !errors.Is(err, ErrNoClientID) {
		t.Fatalf("got %v, want ErrNoClientID", err)
	}
}

func TestDeviceWait(t *testing.T) {
	pending := map[string]any{"error": "authorization_pending"}
	tests := []struct {
		name       string
		polls      []map[string]any
		wantToken  string
		wantErr    error
		wantSleeps []time.Duration
	}{
		{
			name:       "pending then token",
			polls:      []map[string]any{pending, pending, {"access_token": "gho_abc", "token_type": "bearer"}},
			wantToken:  "gho_abc",
			wantSleeps: []time.Duration{5 * time.Second, 5 * time.Second, 5 * time.Second},
		},
		{
			name:       "slow down adds 5 s for all later polls",
			polls:      []map[string]any{{"error": "slow_down"}, pending, {"access_token": "gho_abc"}},
			wantToken:  "gho_abc",
			wantSleeps: []time.Duration{5 * time.Second, 10 * time.Second, 10 * time.Second},
		},
		{
			name:       "slow down honours a larger server interval",
			polls:      []map[string]any{{"error": "slow_down", "interval": 15}, {"access_token": "gho_abc"}},
			wantToken:  "gho_abc",
			wantSleeps: []time.Duration{5 * time.Second, 15 * time.Second},
		},
		{
			name:       "expired token",
			polls:      []map[string]any{pending, {"error": "expired_token"}},
			wantErr:    ErrCodeExpired,
			wantSleeps: []time.Duration{5 * time.Second, 5 * time.Second},
		},
		{
			name:       "access denied",
			polls:      []map[string]any{{"error": "access_denied"}},
			wantErr:    ErrAccessDenied,
			wantSleeps: []time.Duration{5 * time.Second},
		},
		{
			name:  "local expiry stops polling",
			polls: []map[string]any{pending},
			// 900 s / 5 s = 180 polls, then the code is expired locally.
			wantErr: ErrCodeExpired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeOAuth{t: t, polls: tt.polls}
			flow, sleeps := testFlow(fake.server())
			dc, err := flow.Start(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			token, err := flow.Wait(context.Background(), dc)
			if !errors.Is(err, tt.wantErr) || token != tt.wantToken {
				t.Fatalf("Wait = %q, %v; want %q, %v", token, err, tt.wantToken, tt.wantErr)
			}
			if tt.wantSleeps != nil && !equalDurations(*sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v, want %v", *sleeps, tt.wantSleeps)
			}
			if fake.lastForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" ||
				fake.lastForm.Get("device_code") != "dev-123" {
				t.Errorf("poll form = %v", fake.lastForm)
			}
		})
	}
}

func TestDeviceWaitUnknownErrorAndCancel(t *testing.T) {
	fake := &fakeOAuth{t: t, polls: []map[string]any{{"error": "incorrect_client_credentials", "error_description": "bad id"}}}
	flow, _ := testFlow(fake.server())
	dc, _ := flow.Start(context.Background())
	if _, err := flow.Wait(context.Background(), dc); err == nil || errors.Is(err, ErrCodeExpired) {
		t.Fatalf("got %v, want a plain error", err)
	}

	flow.sleep = sleepCtx
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := flow.Wait(ctx, dc); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
