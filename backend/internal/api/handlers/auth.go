package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/mohityadav8/job-find/backend/internal/api/middleware"
	"github.com/mohityadav8/job-find/backend/internal/db"
)

// AuthHandlers implements registration and login for the company dashboard
// (README §3 Phase 2). Passwords are bcrypt-hashed; sessions are stateless JWTs.
type AuthHandlers struct {
	Store *db.Store
	Auth  *middleware.Authenticator
}

// NewAuthHandlers builds auth handlers.
func NewAuthHandlers(store *db.Store, auth *middleware.Authenticator) *AuthHandlers {
	return &AuthHandlers{Store: store, Auth: auth}
}

type authRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	CompanyName string `json:"company_name,omitempty"` // optional at register
}

type authResponse struct {
	Token     string `json:"token"`
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CompanyID *int64 `json:"company_id,omitempty"`
}

// Register handles POST /api/auth/register. If company_name is supplied, a
// company is created and linked to the new user in one step so they can start
// posting offices/jobs immediately.
func (a *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email and a password of 8+ characters are required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Optionally create + link a company.
	var companyID *int64
	if name := strings.TrimSpace(req.CompanyName); name != "" {
		id, err := a.Store.UpsertCompany(r.Context(), name, "", "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create company")
			return
		}
		companyID = &id
	}

	user, err := a.Store.CreateUser(r.Context(), req.Email, string(hash), companyID, "company")
	if err != nil {
		// Most likely a duplicate email (unique constraint).
		writeError(w, http.StatusConflict, "an account with that email already exists")
		return
	}

	token, err := a.Auth.Issue(user.ID, user.Role, user.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusCreated, authResponse{
		Token: token, UserID: user.ID, Email: user.Email,
		Role: user.Role, CompanyID: user.CompanyID,
	})
}

// Login handles POST /api/auth/login.
func (a *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	user, err := a.Store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		// Deliberately vague to avoid revealing which accounts exist.
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := a.Auth.Issue(user.ID, user.Role, user.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{
		Token: token, UserID: user.ID, Email: user.Email,
		Role: user.Role, CompanyID: user.CompanyID,
	})
}

// Me handles GET /api/auth/me — echoes the authenticated user's claims. Useful
// for the frontend to hydrate session state on page load.
func (a *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	uid, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	resp := map[string]any{"user_id": uid}
	if cid, ok := middleware.CompanyIDFromContext(r.Context()); ok {
		resp["company_id"] = cid
	}
	writeJSON(w, http.StatusOK, resp)
}
