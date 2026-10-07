package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func postLogout(r *gin.Engine, header, cookie *string, query, protocol string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/logout"+query, nil)
	request.Header.Set("x-forwarded-proto", protocol)
	if header != nil {
		request.Header.Set("x-refresh-token", *header)
	}
	if cookie != nil {
		request.AddCookie(&http.Cookie{Name: "refresh_token", Value: *cookie})
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}

func assertLogout(t *testing.T, response *httptest.ResponseRecorder, status int, secure bool) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("logout: status=%d body=%s", response.Code, response.Body)
	}
	if status == http.StatusOK && response.Body.String() != `{"redirect_url":null}` {
		t.Fatalf("unexpected logout response: %s", response.Body)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one clearing cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "refresh_token" || cookie.Value != "" || cookie.Path != "/" ||
		cookie.MaxAge != -1 || !cookie.Expires.Before(time.Now()) || !cookie.HttpOnly ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Secure != secure {
		t.Fatalf("unexpected clearing cookie: %+v", cookie)
	}
}

func TestLogoutRevokesOnlyCurrentSession(t *testing.T) {
	db, r := loginFixture(t)
	access, refresh := loginTokens(t, r)
	_, otherRefresh := loginTokens(t, r)
	assertLogout(t, postLogout(r, nil, &refresh, "", "https"), 200, true)
	if sessionCount(t, db) != 1 {
		t.Fatal("normal logout must delete exactly the matching session")
	}
	assertRefreshError(t, postRefresh(r, &refresh, "", ""), 401, "Invalid refresh token")
	assertIdentity(t, getMe(r, "Bearer "+access, "/api/me"), "Ryan", "user")
	assertLogout(t, postLogout(r, nil, &refresh, "", ""), 200, false)
	if sessionCount(t, db) != 1 {
		t.Fatal("repeated logout deleted an unrelated session")
	}
	if response := postRefresh(r, &otherRefresh, "", ""); response.Code != http.StatusOK {
		t.Fatalf("other session must still refresh: %d %s", response.Code, response.Body)
	}
	health := httptest.NewRecorder()
	r.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || health.Body.String() != `{"status":"ok"}` {
		t.Fatal("health stopped working after logout")
	}
	// Login remains available after ending the session.
	loginTokens(t, r)
}

func TestLogoutTransportAndNoop(t *testing.T) {
	for _, tc := range []struct {
		name, header, cookie string
		deleted              bool
	}{
		{"header only", "refresh", "absent", true},
		{"cookie only", "absent", "refresh", true},
		{"cookie wins", "invalid", "refresh", true},
		{"nonempty invalid cookie wins", "refresh", "invalid", false},
		{"empty cookie falls back", "refresh", "", true},
		{"missing", "absent", "absent", false},
		{"empty", "", "", false},
		{"unknown", "unknown", "absent", false},
		{"malformed", "not-a-jwt", "absent", false},
		{"tampered", "tampered", "absent", false},
		{"access token", "access", "absent", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := loginFixture(t)
			access, refresh := loginTokens(t, r)
			resolve := func(value string) *string {
				switch value {
				case "absent":
					return nil
				case "refresh":
					value = refresh
				case "access":
					value = access
				case "tampered":
					value = refresh + "x"
				}
				return &value
			}
			assertLogout(t, postLogout(r, resolve(tc.header), resolve(tc.cookie), "", ""), 200, false)
			want := 1
			if tc.deleted {
				want = 0
			}
			if sessionCount(t, db) != want {
				t.Fatalf("expected %d remaining sessions", want)
			}
		})
	}
}

func TestLogoutAllDevices(t *testing.T) {
	db, r := loginFixture(t)
	access, refresh := loginTokens(t, r)
	_, otherRefresh := loginTokens(t, r)
	root := postLogin(r, `{"username":"root","password":""}`, "application/json", "true", "", "/login")
	var payload struct {
		User struct{ RefreshToken string }
	}
	if err := json.Unmarshal(root.Body.Bytes(), &payload); err != nil || root.Code != 200 || payload.User.RefreshToken == "" {
		t.Fatalf("root login failed: %d %s", root.Code, root.Body)
	}
	// An unknown credential must never select a user for all-device deletion.
	invalid := "unknown-token"
	assertLogout(t, postLogout(r, &invalid, nil, "?allDevices=1", ""), 200, false)
	assertLogout(t, postLogout(r, nil, nil, "?allDevices=1", ""), 200, false)
	if sessionCount(t, db) != 3 {
		t.Fatal("unknown/missing token changed sessions")
	}
	assertLogout(t, postLogout(r, &refresh, nil, "?allDevices=1", ""), 200, false)
	assertLogout(t, postLogout(r, &refresh, nil, "?allDevices=1", ""), 200, false)
	if sessionCount(t, db) != 1 {
		t.Fatal("all-device logout must preserve other users' sessions")
	}
	for _, token := range []string{refresh, otherRefresh} {
		assertRefreshError(t, postRefresh(r, &token, "", ""), 401, "Invalid refresh token")
	}
	assertIdentity(t, getMe(r, "Bearer "+access, "/api/me"), "Ryan", "user")
	if response := postRefresh(r, &payload.User.RefreshToken, "", ""); response.Code != 200 {
		t.Fatal("other user's refresh session was revoked")
	}
}

func TestLogoutOnlyExactAllDevicesFlag(t *testing.T) {
	db, r := loginFixture(t)
	_, refresh := loginTokens(t, r)
	loginTokens(t, r)
	assertLogout(t, postLogout(r, &refresh, nil, "?allDevices=true", ""), 200, false)
	if sessionCount(t, db) != 1 {
		t.Fatal("only allDevices=1 may delete all sessions")
	}
}

func TestLogoutStoredExpiredSession(t *testing.T) {
	db, r := loginFixture(t)
	_, refresh := loginTokens(t, r)
	if _, err := db.Exec(`UPDATE sessions SET expiresAt = '2000-01-01 00:00:00.000 +00:00'`); err != nil {
		t.Fatal(err)
	}
	assertLogout(t, postLogout(r, &refresh, nil, "", ""), 200, false)
	if sessionCount(t, db) != 0 {
		t.Fatal("logout should remove a matching expired session")
	}
}

func TestLogoutStorageFailure(t *testing.T) {
	db, r := loginFixture(t)
	_, refresh := loginTokens(t, r)
	if _, err := db.Exec(`CREATE TRIGGER refuse_logout BEFORE DELETE ON sessions
		BEGIN SELECT RAISE(ABORT, 'fixture logout failure'); END`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "?allDevices=1"} {
		response := postLogout(r, &refresh, nil, query, "")
		assertLogout(t, response, 500, false)
		if response.Body.String() != `{"error":"Logout failed"}` || sessionCount(t, db) != 1 {
			t.Fatal("storage failure must remain generic and preserve the session")
		}
	}
}
