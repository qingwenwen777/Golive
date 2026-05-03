package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	userv1 "github.com/qingwenwen777/golive/api/gen/go/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
)

const LivePermissionApproved = "approved"

type LivePermissionChecker interface {
	HasApprovedLivePermission(ctx context.Context, userID string) (bool, error)
}

type UserPermissionClient struct {
	grpcClient userv1.UserServiceClient
	grpcConn   *grpc.ClientConn
	baseURL    string
	client     *http.Client
	timeout    time.Duration
}

func NewUserPermissionClient(grpcAddr, fallbackURL string) (*UserPermissionClient, error) {
	c := &UserPermissionClient{
		baseURL: strings.TrimRight(fallbackURL, "/"),
		client:  &http.Client{Timeout: 3 * time.Second},
		timeout: 3 * time.Second,
	}
	grpcAddr = strings.TrimSpace(grpcAddr)
	if grpcAddr == "" {
		return c, nil
	}
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	c.grpcConn = conn
	c.grpcClient = userv1.NewUserServiceClient(conn)
	return c, nil
}

func (c *UserPermissionClient) Close() error {
	if c == nil || c.grpcConn == nil {
		return nil
	}
	return c.grpcConn.Close()
}

func (c *UserPermissionClient) HasApprovedLivePermission(ctx context.Context, userID string) (bool, error) {
	if c != nil && c.grpcClient != nil {
		return c.hasApprovedLivePermissionGRPC(ctx, userID)
	}
	return c.hasApprovedLivePermissionHTTP(ctx, userID)
}

func (c *UserPermissionClient) hasApprovedLivePermissionGRPC(ctx context.Context, userID string) (bool, error) {
	if c == nil || c.grpcClient == nil {
		return false, fmt.Errorf("user service grpc client is empty")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.grpcClient.GetUserPermission(callCtx, &userv1.GetUserPermissionRequest{UserId: userID})
	if grpcstatus.Code(err) == codes.NotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return resp.GetLivePermissionApproved(), nil
}

func (c *UserPermissionClient) hasApprovedLivePermissionHTTP(ctx context.Context, userID string) (bool, error) {
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
