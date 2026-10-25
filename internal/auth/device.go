package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"prism/internal/gh"
)

// ClientID is the org-owned OAuth App's client ID (docs/github-oauth-app-setup.md).
// It is not secret. Set it here once the app is registered, or at build time:
// -ldflags "-X prism/internal/auth.ClientID=Ov23...".
var ClientID = ""

// Scopes PRism requests. PRism only reads (github-api skill).
var Scopes = []string{"repo", "notifications", "read:org"}

var (
	ErrNoClientID   = errors.New("auth: no OAuth client ID configured (see docs/github-oauth-app-setup.md)")
	ErrCodeExpired  = errors.New("auth: device code expired")
	ErrAccessDenied = errors.New("auth: access denied")
)

// DeviceCode is what the user needs to authorize PRism in the browser.
type DeviceCode struct {
	UserCode        string
	VerificationURI string
	ExpiresAt       time.Time
	Interval        time.Duration

	deviceCode string
}

// DeviceFlow runs the OAuth 2.0 device authorization grant against GitHub.
// The poll loop is our own instead of x/oauth2's so the clock and sleeps can
// be faked in tests and the UI can show progress.
type DeviceFlow struct {
	ClientID  string
	Endpoints gh.Endpoints
	UserAgent string
	HTTP      *http.Client

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// NewDeviceFlow returns a DeviceFlow for github.com-style endpoints.
func NewDeviceFlow(clientID string, ep gh.Endpoints, version string) *DeviceFlow {
	return &DeviceFlow{
		ClientID:  clientID,
		Endpoints: ep,
		UserAgent: "PRism/" + version,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		now:       time.Now,
		sleep:     sleepCtx,
	}
}

// Start requests a device and user code.
func (f *DeviceFlow) Start(ctx context.Context) (*DeviceCode, error) {
	if f.ClientID == "" {
		return nil, ErrNoClientID
	}
	var resp struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		oauthError
	}
	form := url.Values{"client_id": {f.ClientID}, "scope": {strings.Join(Scopes, " ")}}
	if err := f.post(ctx, f.Endpoints.DeviceCodeURL(), form, &resp); err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("request device code: %w", resp.err())
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return nil, errors.New("request device code: incomplete response")
	}
	interval := time.Duration(resp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second // RFC 8628 §3.2 default
	}
	return &DeviceCode{
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		ExpiresAt:       f.now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		Interval:        interval,
		deviceCode:      resp.DeviceCode,
	}, nil
}

// Wait polls until the user authorizes PRism and returns the access token.
// It returns ErrCodeExpired or ErrAccessDenied for the two terminal user outcomes.
func (f *DeviceFlow) Wait(ctx context.Context, dc *DeviceCode) (string, error) {
	interval := dc.Interval
	form := url.Values{
		"client_id":   {f.ClientID},
		"device_code": {dc.deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	for {
		if !f.now().Before(dc.ExpiresAt) {
			return "", ErrCodeExpired
		}
		if err := f.sleep(ctx, interval); err != nil {
			return "", err
		}

		var resp struct {
			AccessToken string `json:"access_token"`
			Interval    int    `json:"interval"`
			oauthError
		}
		if err := f.post(ctx, f.Endpoints.AccessTokenURL(), form, &resp); err != nil {
			return "", fmt.Errorf("poll access token: %w", err)
		}
		switch resp.Error {
		case "":
			if resp.AccessToken == "" {
				return "", errors.New("poll access token: empty token")
			}
			return resp.AccessToken, nil
		case "authorization_pending":
		case "slow_down":
			// RFC 8628 §3.5: add 5 s; GitHub also sends the new interval.
			interval += 5 * time.Second
			if s := time.Duration(resp.Interval) * time.Second; s > interval {
				interval = s
			}
		case "expired_token":
			return "", ErrCodeExpired
		case "access_denied":
			return "", ErrAccessDenied
		default:
			return "", fmt.Errorf("poll access token: %w", resp.err())
		}
	}
}

type oauthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (e oauthError) err() error {
	if e.ErrorDescription != "" {
		return fmt.Errorf("%s: %s", e.Error, e.ErrorDescription)
	}
	return errors.New(e.Error)
}

func (f *DeviceFlow) post(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", f.UserAgent)
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	// GitHub answers OAuth errors with 200 + {"error": ...}; other statuses
	// carry a JSON error too, so decode first and judge the status after.
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("HTTP %d: undecodable response", resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
