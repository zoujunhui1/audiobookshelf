package services

import "context"

// Logout treats the stored refresh token as the session credential, matching
// Node's local logout. Expired credentials can still delete their stored session;
// missing, unknown and already-deleted tokens are harmless. Access JWTs survive.
func (s *AuthService) Logout(ctx context.Context, value string, allDevices bool) error {
	if value == "" {
		return nil
	}
	return s.sessions.InvalidateByRefreshToken(ctx, value, allDevices)
}
