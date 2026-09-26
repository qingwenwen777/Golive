package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const srsAPITimeout = 3 * time.Second

// srsAPI calls the SRS HTTP API (http_api in deploy/srs.conf, port 1985).
type srsAPI struct {
	base   string
	client *http.Client
}

func newSRSAPI(base string) *srsAPI {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil
	}
	return &srsAPI{base: base, client: &http.Client{Timeout: srsAPITimeout}}
}

// kickClient disconnects the SRS client with the id SRS reported as client_id
// in its hooks. SRS 5 answers DELETE /api/v1/clients/{id} with HTTP 200 and a
// JSON "code"; a non-zero code (e.g. client not found) means nothing was kicked.
func (a *srsAPI) kickClient(ctx context.Context, clientID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.base+"/api/v1/clients/"+url.PathEscape(clientID), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("srs kick client: http %d", resp.StatusCode)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return fmt.Errorf("srs kick client: decode response: %w", err)
	}
	if body.Code != 0 {
		return fmt.Errorf("srs kick client: code %d", body.Code)
	}
	return nil
}
