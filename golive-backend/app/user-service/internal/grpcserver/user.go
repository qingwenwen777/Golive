package grpcserver

import (
	"context"
	"errors"
	"strings"

	userv1 "github.com/qingwenwen777/golive/api/gen/go/user/v1"
	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserServer struct {
	users *repo.UserRepo
}

func NewUserServer(users *repo.UserRepo) *UserServer {
	return &UserServer{users: users}
}

func (s *UserServer) GetUserPermission(ctx context.Context, req *userv1.GetUserPermissionRequest) (*userv1.GetUserPermissionResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	u, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, repo.ErrUserNotFound) {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "find user")
	}
	role := u.Role
	if role == "" {
		role = model.RoleUser
	}
	liveStatus := u.LivePermissionStatus
	if liveStatus == "" {
		liveStatus = model.LivePermissionNone
	}
	return &userv1.GetUserPermissionResponse{
		UserId:                 u.ID,
		Role:                   role,
		LivePermissionStatus:   liveStatus,
		LivePermissionApproved: liveStatus == model.LivePermissionApproved,
	}, nil
}
