package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const LivePermissionApproved = "approved"

type LivePermissionChecker interface {
	HasApprovedLivePermission(ctx context.Context, userID string) (bool, error)
}

type UserPermissionClient struct {
	baseURL string
	client  *http.Client
}

func NewUserPermissionClient(baseURL string) *UserPermissionClient {
	return &UserPermissionClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *UserPermissionClient) HasApprovedLivePermission(ctx context.Context, userID string) (bool, error) {
	if c == nil || c.baseURL == "" {
		return false, fmt.Errorf("user service url is empty")
	}
	endpoint := c.baseURL + "/internal/users/" + url.PathEscape(userID) + "/permission"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("user service returned %d", resp.StatusCode)
	}
	var body struct {
		LivePermissionStatus string `json:"livePermissionStatus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, err
	}
	return body.LivePermissionStatus == LivePermissionApproved, nil
}
