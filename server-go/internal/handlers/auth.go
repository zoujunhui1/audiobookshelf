package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf-go/internal/repository"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Auth handles POST /login using repository.GetUserByUsername, bcrypt, and JWT.
// JSON shape matches server/Auth.js handleLoginSuccess / getUserLoginResponsePayload.
type Auth struct {
	Users     *repository.UserRepo
	JWTSecret []byte
}

func (h *Auth) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.Users.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	if user == nil {
		// passport.authenticate('local') fails with 401 when the user is missing
		c.String(http.StatusUnauthorized, "Unauthorized")
		return
	}

	if !user.IsActive || !passwordMatches(user, req.Password) {
		c.String(http.StatusUnauthorized, "Unauthorized")
		return
	}

	accessToken, err := h.signToken(user, "access", time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	var refreshToken any
	if c.GetHeader("x-return-tokens") == "true" {
		rt, err := h.signToken(user, "refresh", 30*24*time.Hour)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		refreshToken = rt
	}

	c.JSON(http.StatusOK, loginSuccessPayload(user, accessToken, refreshToken))
}

func passwordMatches(user *repository.User, password string) bool {
	if user.Type == "root" && (!user.Pash.Valid || user.Pash.String == "") {
		return password == ""
	}
	if !user.Pash.Valid || user.Pash.String == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(user.Pash.String), []byte(password))
	return err == nil
}

func (h *Auth) signToken(user *repository.User, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"userId":   user.ID,
		"username": user.Username,
		"type":     tokenType,
		"iat":      now.Unix(),
		"exp":      now.Add(ttl).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.JWTSecret)
}

func loginSuccessPayload(user *repository.User, accessToken string, refreshToken any) gin.H {
	return gin.H{
		"user":                 userToBrowserJSON(user, accessToken, refreshToken),
		"userDefaultLibraryId": nil,
		"serverSettings":       serverSettingsJSON(),
		"ereaderDevices":       []any{},
		"Source":               "local",
	}
}

func userToBrowserJSON(user *repository.User, accessToken string, refreshToken any) gin.H {
	permissions, librariesAccessible, itemTagsSelected := parsePermissions(user.Permissions)
	bookmarks := parseJSONArray(user.Bookmarks)
	seriesHide, hasOpenIDLink := parseExtraData(user.ExtraData)

	var email any
	if user.Email.Valid {
		email = user.Email.String
	}

	var lastSeen any
	if user.LastSeen.Valid {
		lastSeen = user.LastSeen.Time.UnixMilli()
	}

	token := accessToken
	if user.Token.Valid && user.Token.String != "" {
		token = user.Token.String
	}

	return gin.H{
		"id":                              user.ID,
		"username":                        user.Username,
		"email":                           email,
		"type":                            user.Type,
		"token":                           token,
		"isOldToken":                      false,
		"mediaProgress":                   []any{},
		"seriesHideFromContinueListening": seriesHide,
		"bookmarks":                       bookmarks,
		"isActive":                        user.IsActive,
		"isLocked":                        user.IsLocked,
		"lastSeen":                        lastSeen,
		"createdAt":                       user.CreatedAt.UnixMilli(),
		"permissions":                     permissions,
		"librariesAccessible":             librariesAccessible,
		"itemTagsSelected":                itemTagsSelected,
		"hasOpenIDLink":                   hasOpenIDLink,
		"accessToken":                     accessToken,
		"refreshToken":                    refreshToken,
	}
}

func parsePermissions(raw []byte) (gin.H, []any, []any) {
	permissions := gin.H{
		"download":                  true,
		"update":                    true,
		"delete":                    true,
		"upload":                    true,
		"createEreader":             true,
		"accessAllLibraries":        true,
		"accessAllTags":             true,
		"accessExplicitContent":     true,
		"selectedTagsNotAccessible": false,
	}
	librariesAccessible := []any{}
	itemTagsSelected := []any{}

	if len(raw) == 0 {
		return permissions, librariesAccessible, itemTagsSelected
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return permissions, librariesAccessible, itemTagsSelected
	}

	if v, ok := parsed["librariesAccessible"].([]any); ok {
		librariesAccessible = v
	}
	if v, ok := parsed["itemTagsSelected"].([]any); ok {
		itemTagsSelected = v
	}
	delete(parsed, "librariesAccessible")
	delete(parsed, "itemTagsSelected")
	for k, v := range parsed {
		permissions[k] = v
	}
	return permissions, librariesAccessible, itemTagsSelected
}

func parseJSONArray(raw []byte) []any {
	out := []any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return []any{}
	}
	return out
}

func parseExtraData(raw []byte) ([]any, bool) {
	seriesHide := []any{}
	if len(raw) == 0 {
		return seriesHide, false
	}
	var extra map[string]any
	if err := json.Unmarshal(raw, &extra); err != nil {
		return seriesHide, false
	}
	if v, ok := extra["seriesHideFromContinueListening"].([]any); ok {
		seriesHide = v
	}
	sub, _ := extra["authOpenIDSub"].(string)
	return seriesHide, sub != ""
}

func serverSettingsJSON() gin.H {
	return gin.H{
		"id":                                 "server-settings",
		"scannerFindCovers":                  false,
		"scannerCoverProvider":               "google",
		"scannerParseSubtitle":               false,
		"scannerPreferMatchedMetadata":       false,
		"scannerDisableWatcher":              false,
		"storeCoverWithItem":                 false,
		"storeMetadataWithItem":              false,
		"metadataFileFormat":                 "json",
		"rateLimitLoginRequests":             10,
		"rateLimitLoginWindow":               600000,
		"allowIframe":                        false,
		"backupPath":                         "",
		"backupSchedule":                     false,
		"backupsToKeep":                      2,
		"maxBackupSize":                      1,
		"loggerDailyLogsToKeep":              7,
		"loggerScannerLogsToKeep":            2,
		"homeBookshelfView":                  1,
		"bookshelfView":                      1,
		"podcastEpisodeSchedule":             "0 * * * *",
		"sortingIgnorePrefix":                false,
		"sortingPrefixes":                    []string{"the", "a"},
		"chromecastEnabled":                  false,
		"dateFormat":                         "MM/dd/yyyy",
		"timeFormat":                         "HH:mm",
		"language":                           "en-us",
		"allowedOrigins":                     []any{},
		"logLevel":                           2,
		"version":                            "2.37.1",
		"buildNumber":                        1,
		"authLoginCustomMessage":             nil,
		"authActiveAuthMethods":              []string{"local"},
		"authOpenIDIssuerURL":                nil,
		"authOpenIDAuthorizationURL":         nil,
		"authOpenIDTokenURL":                 nil,
		"authOpenIDUserInfoURL":              nil,
		"authOpenIDJwksURL":                  nil,
		"authOpenIDLogoutURL":                nil,
		"authOpenIDTokenSigningAlgorithm":    "RS256",
		"authOpenIDButtonText":               "Login with OpenId",
		"authOpenIDAutoLaunch":               false,
		"authOpenIDAutoRegister":             false,
		"authOpenIDMatchExistingBy":          nil,
		"authOpenIDSubfolderForRedirectURLs": nil,
		"timeZone":                           "UTC",
	}
}
