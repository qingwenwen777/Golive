package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultCloudflareAPIBase = "https://api.cloudflare.com/client/v4"

type CloudflareStreamConfig struct {
	AccountID     string
	StreamToken   string
	APIBase       string
	RecordingMode string
	HTTPTimeout   time.Duration
}

type CloudflareStreamClient struct {
	accountID     string
	streamToken   string
	apiBase       string
	recordingMode string
	httpClient    *http.Client
}

type CloudflareLiveInput struct {
	UID         string
	RTMPServer  string
	StreamKey   string
	PlaybackURL string
}

type cloudflareEnvelope[T any] struct {
	Result   T                    `json:"result"`
	Success  bool                 `json:"success"`
	Errors   []cloudflareAPIError `json:"errors"`
	Messages []cloudflareAPIError `json:"messages"`
}

type cloudflareAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareLiveInputResult struct {
	UID            string         `json:"uid"`
	RTMPS          cloudflareRTMP `json:"rtmps"`
	WebRTC         cloudflareRTC  `json:"webRTC"`
	WebRTCPlayback cloudflareRTC  `json:"webRTCPlayback"`
}

type cloudflareRTMP struct {
	URL       string `json:"url"`
	StreamKey string `json:"streamKey"`
}

type cloudflareRTC struct {
	URL         string `json:"url"`
	PlaybackURL string `json:"playbackUrl"`
}

func NewCloudflareStreamClient(cfg CloudflareStreamConfig) *CloudflareStreamClient {
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = defaultCloudflareAPIBase
	}
	recordingMode := strings.TrimSpace(cfg.RecordingMode)
	if recordingMode == "" {
		recordingMode = "off"
	}
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &CloudflareStreamClient{
		accountID:     strings.TrimSpace(cfg.AccountID),
		streamToken:   strings.TrimSpace(cfg.StreamToken),
		apiBase:       apiBase,
		recordingMode: recordingMode,
		httpClient:    &http.Client{Timeout: timeout},
	}
}

func (c *CloudflareStreamClient) Enabled() bool {
	return c != nil && c.accountID != "" && c.streamToken != ""
}

func (c *CloudflareStreamClient) CreateLiveInput(ctx context.Context, name string) (*CloudflareLiveInput, error) {
	if !c.Enabled() {
		return nil, errors.New("cloudflare stream is not configured")
	}
	body := map[string]any{
		"enabled": true,
		"meta": map[string]string{
			"name": strings.TrimSpace(name),
		},
		"recording": map[string]any{
			"mode":                c.recordingMode,
			"requireSignedURLs":   false,
			"hideLiveViewerCount": false,
			"timeoutSeconds":      0,
		},
	}
	var envelope cloudflareEnvelope[cloudflareLiveInputResult]
	if err := c.do(ctx, http.MethodPost, "/stream/live_inputs", body, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("cloudflare create live input: %s", formatCloudflareErrors(envelope.Errors))
	}
	result := envelope.Result
	playbackURL := cloudflareIframeURL(
		result.UID,
		result.WebRTCPlayback.URL,
		result.WebRTCPlayback.PlaybackURL,
		result.WebRTC.URL,
		result.WebRTC.PlaybackURL,
	)
	return &CloudflareLiveInput{
		UID:         result.UID,
		RTMPServer:  result.RTMPS.URL,
		StreamKey:   result.RTMPS.StreamKey,
		PlaybackURL: playbackURL,
	}, nil
}

func (c *CloudflareStreamClient) DeleteLiveInput(ctx context.Context, uid string) error {
	if !c.Enabled() || strings.TrimSpace(uid) == "" {
		return nil
	}
	var envelope cloudflareEnvelope[json.RawMessage]
	if err := c.do(ctx, http.MethodDelete, "/stream/live_inputs/"+url.PathEscape(uid), nil, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("cloudflare delete live input: %s", formatCloudflareErrors(envelope.Errors))
	}
	return nil
}

func (c *CloudflareStreamClient) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+"/accounts/"+url.PathEscape(c.accountID)+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.streamToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode cloudflare response: %w", err)
	}
	return nil
}

func cloudflareIframeURL(uid string, candidates ...string) string {
	if strings.TrimSpace(uid) == "" {
		return ""
	}
	for _, raw := range candidates {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		return u.Scheme + "://" + u.Host + "/" + url.PathEscape(uid) + "/iframe"
	}
	return ""
}

func formatCloudflareErrors(errors []cloudflareAPIError) string {
	if len(errors) == 0 {
		return "unknown error"
	}
	parts := make([]string, 0, len(errors))
	for _, item := range errors {
		if item.Code != 0 {
			parts = append(parts, fmt.Sprintf("%d %s", item.Code, item.Message))
		} else {
			parts = append(parts, item.Message)
		}
	}
	return strings.Join(parts, "; ")
}
