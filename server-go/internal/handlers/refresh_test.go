package handlers_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func postRefresh(r *gin.Engine, header *string, cookie, protocol string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	request.Header.Set("User-Agent", "refresh-test")
	request.Header.Set("x-forwarded-proto", protocol)
	if header != nil {
		request.Header.Set("x-refresh-token", *header)
	}
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: "refresh_token", Value: cookie})
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}

func assertRefreshError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != status || len(payload) != 1 || payload["error"] != message || len(response.Result().Cookies()) != 0 {
		t.Fatalf("unexpected refresh failure or returned credentials: %d %s", response.Code, response.Body)
	}
}

func checkRefreshSuccess(t *testing.T, db *sql.DB, response *httptest.ResponseRecorder,
	oldAccess, oldRefresh string, headerMode, secure bool) (string, string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("refresh failed: %d %s", response.Code, response.Body)
	}
	var payload struct {
		User struct {
			ID, Username, Type, AccessToken string
			IsActive                        bool
			RefreshToken                    *string
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.User.ID != "active-id" || payload.User.Username != "Ryan" || payload.User.Type != "user" || !payload.User.IsActive {
		t.Fatal("refresh did not return the current basic identity")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("refresh must set a cookie in both transport modes")
	}
	cookie := cookies[0]
	if cookie.Name != "refresh_token" || cookie.Path != "/" || !cookie.HttpOnly ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Secure != secure || cookie.MaxAge != 30*24*3600 {
		t.Fatalf("unexpected refresh cookie: %+v", cookie)
	}
	if headerMode {
		if payload.User.RefreshToken == nil || *payload.User.RefreshToken != cookie.Value {
			t.Fatal("header mode must return the same token in JSON and the cookie")
		}
	} else if payload.User.RefreshToken != nil {
		t.Fatal("cookie mode must keep user.refreshToken null")
	}
	access, refresh := payload.User.AccessToken, cookie.Value
	if access == oldAccess || refresh == oldRefresh {
		t.Fatal("refresh must issue a new access token and rotate the refresh token")
	}
	tokenClaims(t, access, "access", payload.User.ID)
	claims := tokenClaims(t, refresh, "refresh", payload.User.ID)
	var stored, expiry string
	var previous, previousExpiry sql.NullString
	if err := db.QueryRow(`SELECT refreshToken, CAST(expiresAt AS TEXT), lastRefreshToken,
		lastRefreshTokenExpiresAt FROM sessions WHERE userId = 'active-id'`).Scan(
		&stored, &expiry, &previous, &previousExpiry,
	); err != nil {
		t.Fatal(err)
	}
	expiresAt, err := time.Parse("2006-01-02 15:04:05.000 -07:00", expiry)
	if err != nil || expiresAt.Unix() != int64(claims["exp"].(float64)) || stored != refresh || previous.Valid || previousExpiry.Valid {
		t.Fatal("stored session does not match the rotated token/expiry")
	}
	if !cookie.Expires.Equal(time.Unix(int64(claims["exp"].(float64)), 0).UTC()) {
		t.Fatal("cookie and JWT expiry disagree")
	}
	if sessionCount(t, db) != 1 {
		t.Fatal("refresh must update the session rather than insert another one")
	}
	return access, refresh
}

func TestRefreshHeaderRotation(t *testing.T) {
	db, r := loginFixture(t)
	oldAccess, oldRefresh := loginTokens(t, r)
	var id, created, ip, agent string
	if err := db.QueryRow(`SELECT id, CAST(createdAt AS TEXT), ipAddress, userAgent FROM sessions`).Scan(&id, &created, &ip, &agent); err != nil {
		t.Fatal(err)
	}
	response := postRefresh(r, &oldRefresh, "invalid-cookie", "")
	access, refresh := checkRefreshSuccess(t, db, response, oldAccess, oldRefresh, true, false)
	var rotatedID, rotatedCreated, rotatedIP, rotatedAgent, updated string
	if err := db.QueryRow(`SELECT id, CAST(createdAt AS TEXT), ipAddress, userAgent, CAST(updatedAt AS TEXT)
		FROM sessions`).Scan(&rotatedID, &rotatedCreated, &rotatedIP, &rotatedAgent, &updated); err != nil {
		t.Fatal(err)
	}
	if rotatedID != id || rotatedCreated != created || rotatedIP != ip || rotatedAgent != agent || updated < created {
		t.Fatal("rotation changed original session identity/metadata")
	}
	assertIdentity(t, getMe(r, "Bearer "+access, "/api/me"), "Ryan", "user")
	// Rotating refresh credentials does not revoke existing access JWTs.
	assertIdentity(t, getMe(r, "Bearer "+oldAccess, "/api/me"), "Ryan", "user")
	assertRefreshError(t, postRefresh(r, &oldRefresh, "", ""), 401, "Invalid refresh token")
	response = postRefresh(r, &refresh, "", "")
	checkRefreshSuccess(t, db, response, access, refresh, true, false)
}

func TestRefreshCookieTransport(t *testing.T) {
	db, r := loginFixture(t)
	login := postLogin(r, `{"username":"Ryan","password":"test-password"}`,
		"application/json", "", "", "http://example.test/login")
	checkSuccessfulLogin(t, db, login, false, false)
	oldRefresh := login.Result().Cookies()[0].Value
	response := postRefresh(r, nil, oldRefresh, "http, https")
	access, refresh := checkRefreshSuccess(t, db, response, "", oldRefresh, false, true)
	assertIdentity(t, getMe(r, "Bearer "+access, "/api/me"), "Ryan", "user")
	assertRefreshError(t, postRefresh(r, nil, oldRefresh, ""), 401, "Invalid refresh token")
	checkRefreshSuccess(t, db, postRefresh(r, nil, refresh, ""), access, refresh, false, false)
}

func TestRefreshRejectedTokens(t *testing.T) {
	db, r := loginFixture(t)
	access, refresh := loginTokens(t, r)
	now := time.Now()
	sign := func(expiry any, userID any, method jwt.SigningMethod, key string) string {
		claims := jwt.MapClaims{"type": "refresh", "userId": userID, "jti": "unknown-token"}
		if expiry != nil {
			claims["exp"] = expiry
		}
		return signedTestToken(t, method, []byte(key), claims)
	}
	for _, tc := range []struct {
		name, value, cookie, message string
	}{
		{"malformed", "not-a-jwt", "", "Invalid refresh token"},
		{"tampered", refresh + "x", "", "Invalid refresh token"},
		{"access token", access, "", "Invalid token type"},
		{"expired JWT", sign(now.Add(-time.Minute).Unix(), "active-id", jwt.SigningMethodHS256, testSecret), "", "Refresh token expired"},
		{"unknown valid token", sign(now.Add(time.Hour).Unix(), "active-id", jwt.SigningMethodHS256, testSecret), "", "Invalid refresh token"},
		{"wrong key", sign(now.Add(time.Hour).Unix(), "active-id", jwt.SigningMethodHS256, "wrong-key"), "", "Invalid refresh token"},
		{"wrong algorithm", sign(now.Add(time.Hour).Unix(), "active-id", jwt.SigningMethodHS512, testSecret), "", "Invalid refresh token"},
		{"missing expiry", sign(nil, "active-id", jwt.SigningMethodHS256, testSecret), "", "Invalid refresh token"},
		{"missing user ID", sign(now.Add(time.Hour).Unix(), "", jwt.SigningMethodHS256, testSecret), "", "Invalid refresh token"},
		{"header overrides valid cookie", "invalid-header", refresh, "Invalid refresh token"},
		{"empty header overrides cookie", "", refresh, "No refresh token provided"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertRefreshError(t, postRefresh(r, &tc.value, tc.cookie, ""), 401, tc.message)
		})
	}
	assertRefreshError(t, postRefresh(r, nil, "", ""), 401, "No refresh token provided")
	var stored string
	if err := db.QueryRow(`SELECT refreshToken FROM sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != refresh || sessionCount(t, db) != 1 {
		t.Fatal("rejected requests changed the stored session")
	}
}

func TestRefreshStoredSessionChecks(t *testing.T) {
	for _, tc := range []struct {
		name, change, message string
	}{
		{"expired session", `UPDATE sessions SET expiresAt = '2000-01-01 00:00:00.000 +00:00'`, "Refresh token expired"},
		{"unknown session", `DELETE FROM sessions`, "Invalid refresh token"},
		{"mismatched user", `UPDATE sessions SET userId = 'root-id'`, "Invalid refresh token"},
		{"inactive user", `UPDATE users SET isActive = 0 WHERE id = 'active-id'`, "User not found or inactive"},
		{"deleted user", `DELETE FROM users WHERE id = 'active-id'`, "Invalid refresh token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := loginFixture(t)
			_, refresh := loginTokens(t, r)
			if _, err := db.Exec(tc.change); err != nil {
				t.Fatal(err)
			}
			before := sessionCount(t, db)
			assertRefreshError(t, postRefresh(r, &refresh, "", ""), 401, tc.message)
			if sessionCount(t, db) != before {
				t.Fatal("rejected refresh changed session count")
			}
		})
	}
}

func TestRefreshRotationFailure(t *testing.T) {
	db, r := loginFixture(t)
	_, refresh := loginTokens(t, r)
	if _, err := db.Exec(`CREATE TRIGGER refuse_rotation BEFORE UPDATE ON sessions
		BEGIN SELECT RAISE(ABORT, 'fixture rotation failure'); END`); err != nil {
		t.Fatal(err)
	}
	assertRefreshError(t, postRefresh(r, &refresh, "", ""), 500, "Refresh failed")
	var stored string
	if err := db.QueryRow(`SELECT refreshToken FROM sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != refresh {
		t.Fatal("failed rotation changed the session")
	}
}

func TestConcurrentRefreshHasOneWinner(t *testing.T) {
	db, r := loginFixture(t)
	access, refresh := loginTokens(t, r)
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			responses <- postRefresh(r, &refresh, "", "")
		}()
	}
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		response := <-responses
		if response.Code == 200 {
			successes++
			checkRefreshSuccess(t, db, response, access, refresh, true, false)
		} else {
			assertRefreshError(t, response, 401, "Invalid refresh token")
		}
	}
	if successes != 1 {
		t.Fatalf("expected one rotation winner, got %d", successes)
	}
}
