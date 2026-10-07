package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func loginTokens(t *testing.T, r *gin.Engine) (string, string) {
	t.Helper()
	response := postLogin(r, `{"username":"Ryan","password":"test-password"}`,
		"application/json", "true", "", "http://example.test/login")
	if response.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", response.Code, response.Body)
	}
	var payload struct {
		User struct{ AccessToken, RefreshToken string }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.User.AccessToken, payload.User.RefreshToken
}

func getMe(r *gin.Engine, header, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", header)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}

func assertIdentity(t *testing.T, response *httptest.ResponseRecorder, username, userType string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/me: %d %s", response.Code, response.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": "active-id", "username": username, "type": userType, "isActive": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected only current identity fields: got %v, want %v", got, want)
	}
}

func signedTestToken(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	value, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestMeAccessTokenAuthentication(t *testing.T) {
	db, r := loginFixture(t)
	access, refresh := loginTokens(t, r)
	for _, scheme := range []string{"Bearer", "bearer"} {
		assertIdentity(t, getMe(r, scheme+" "+access, "/api/me"), "Ryan", "user")
	}
	now := time.Now()
	makeToken := func(kind string, userID any, expiry any) string {
		claims := jwt.MapClaims{"type": kind, "userId": userID}
		if expiry != nil {
			claims["exp"] = expiry
		}
		return signedTestToken(t, jwt.SigningMethodHS256, []byte(testSecret), claims)
	}
	validClaims := jwt.MapClaims{"type": "access", "userId": "active-id", "exp": now.Add(time.Hour).Unix()}
	for _, tc := range []struct {
		name, header, path string
	}{
		{"missing token", "", "/api/me"},
		{"empty bearer token", "Bearer", "/api/me"},
		{"wrong scheme", "Basic " + access, "/api/me"},
		{"extra header fields", "Bearer " + access + " extra", "/api/me"},
		{"malformed token", "Bearer not-a-jwt", "/api/me"},
		{"tampered signature", "Bearer " + access + "x", "/api/me"},
		{"expired token", "Bearer " + makeToken("access", "active-id", now.Add(-time.Minute).Unix()), "/api/me"},
		{"refresh from login", "Bearer " + refresh, "/api/me"},
		{"wrong signing key", "Bearer " + signedTestToken(t, jwt.SigningMethodHS256, []byte("wrong-key"), validClaims), "/api/me"},
		{"wrong algorithm", "Bearer " + signedTestToken(t, jwt.SigningMethodHS512, []byte(testSecret), validClaims), "/api/me"},
		{"unsigned token", "Bearer " + signedTestToken(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims), "/api/me"},
		{"missing expiry", "Bearer " + makeToken("access", "active-id", nil), "/api/me"},
		{"missing access type", "Bearer " + makeToken("", "active-id", now.Add(time.Hour).Unix()), "/api/me"},
		{"missing user ID", "Bearer " + makeToken("access", "", now.Add(time.Hour).Unix()), "/api/me"},
		{"invalid user ID type", "Bearer " + makeToken("access", 1, now.Add(time.Hour).Unix()), "/api/me"},
		{"unknown user", "Bearer " + makeToken("access", "unknown-id", now.Add(time.Hour).Unix()), "/api/me"},
		{"inactive user", "Bearer " + makeToken("access", "inactive-id", now.Add(time.Hour).Unix()), "/api/me"},
		{"parameterized ID lookup", "Bearer " + makeToken("access", "' OR 1=1 --", now.Add(time.Hour).Unix()), "/api/me"},
		{"query token unsupported", "", "/api/me?token=" + access},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := getMe(r, tc.header, tc.path)
			if response.Code != http.StatusUnauthorized || response.Body.String() != `{"error":"Unauthorized"}` ||
				response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("expected only 401 error/challenge: %d %s", response.Code, response.Body)
			}
		})
	}
	if got := sessionCount(t, db); got != 1 {
		t.Fatalf("authentication must not create/rotate sessions: got %d", got)
	}
	// Authentication is attached only to /api/me; /health remains public.
	response := getMe(r, "Bearer invalid", "/health")
	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Fatal("authentication middleware changed /health")
	}
}

func TestMeUsesCurrentRepositoryIdentity(t *testing.T) {
	db, r := loginFixture(t)
	access, _ := loginTokens(t, r)
	if _, err := db.Exec(`UPDATE users SET username = 'Renamed', type = 'admin' WHERE id = 'active-id'`); err != nil {
		t.Fatal(err)
	}
	assertIdentity(t, getMe(r, "Bearer "+access, "/api/me"), "Renamed", "admin")
	if _, err := db.Exec(`UPDATE users SET isActive = 0 WHERE id = 'active-id'`); err != nil {
		t.Fatal(err)
	}
	if response := getMe(r, "Bearer "+access, "/api/me"); response.Code != http.StatusUnauthorized {
		t.Fatalf("deactivated user still authenticated: %d", response.Code)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = 'active-id'`); err != nil {
		t.Fatal(err)
	}
	if response := getMe(r, "Bearer "+access, "/api/me"); response.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user still authenticated: %d", response.Code)
	}
}

func TestMeRepositoryFailure(t *testing.T) {
	db, r := loginFixture(t)
	access, _ := loginTokens(t, r)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	response := getMe(r, "Bearer "+access, "/api/me")
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"Authentication failed"}` {
		t.Fatalf("repository failure leaked identity/internal error or failed open: %d %s", response.Code, response.Body)
	}
}
