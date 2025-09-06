package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	lang_adapters "fulcrum/lib/lang/adapters"

	"github.com/aymerick/raymond"
	"github.com/golang-jwt/jwt/v5"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type User struct {
	Username string
	Password string // In production, this should be hashed
	Id       float64
}

var jwtSecret = []byte("your-secret-key-change-this-in-production")

var users = map[string]User{
	"admin": {Username: "admin", Password: "password123"},
	"user":  {Username: "user", Password: "userpass"},
}

// findAuthTemplate finds an auth template, checking project domains first, then lib/views fallback
func findAuthTemplate(templateName string) (string, error) {
	// Get current working directory for project-specific templates
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check project-specific auth template first
	projectTemplate := filepath.Join(cwd, "domains", "auth", templateName)
	if _, err := os.Stat(projectTemplate); err == nil {
		log.Printf("🎯 Using project-specific auth template: %s", projectTemplate)
		return projectTemplate, nil
	}

	// Fall back to lib/views/auth default template
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get runtime caller info")
	}

	// Navigate from lib/auth/auth.go to lib/views/auth
	libDir := filepath.Dir(filepath.Dir(filename)) // Go up two levels from lib/auth/
	libTemplate := filepath.Join(libDir, "views", "auth", templateName)

	if _, err := os.Stat(libTemplate); err == nil {
		log.Printf("🏷️ Using default auth template: %s", libTemplate)
		return libTemplate, nil
	}

	return "", fmt.Errorf("auth template %s not found in project or lib/views", templateName)
}

// loadAuthTemplate loads and renders an auth template with data
func loadAuthTemplate(templateName string, data map[string]interface{}) (string, error) {
	templatePath, err := findAuthTemplate(templateName)
	if err != nil {
		return "", err
	}

	tmpl, err := raymond.ParseFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templateName, err)
	}

	html, err := tmpl.Exec(data)
	if err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return html, nil
}

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if IsAuthenticated(r) {
		http.Redirect(w, r, "/auth/dashboard", http.StatusSeeOther)
		return
	}

	// Get error/success from query params if any
	errorMsg := r.URL.Query().Get("error")
	successMsg := r.URL.Query().Get("success")

	data := map[string]interface{}{}
	if errorMsg != "" {
		data["error"] = errorMsg
	}
	if successMsg != "" {
		data["success"] = successMsg
	}

	// Try to load dynamic template, fallback to hardcoded if needed
	html, err := loadAuthTemplate("login/get.html.hbs", data)
	if err != nil {
		log.Printf("⚠️ Failed to load dynamic auth template, using fallback: %v", err)
		// Fallback to hardcoded template
		loginTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login</title>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-100 min-h-screen flex items-center justify-center">
    <div class="bg-white p-8 rounded-lg shadow-md w-full max-w-md">
        <h2 class="text-2xl font-bold text-center text-gray-800 mb-6">Login</h2>
        
        {{#if error}}
        <div class="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded mb-4">
            {{error}}
        </div>
        {{/if}}

        {{#if success}}
        <div class="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded mb-4">
            {{success}}
        </div>
        {{/if}}

        <form method="POST" action="/auth/login" class="space-y-4">
            <div>
                <label for="username" class="block text-sm font-medium text-gray-700 mb-1">Email</label>
                <input type="email" id="username" name="username" required 
                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent">
            </div>
            
            <div>
                <label for="password" class="block text-sm font-medium text-gray-700 mb-1">Password</label>
                <input type="password" id="password" name="password" required 
                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent">
            </div>
            
            <button type="submit" 
                    class="w-full bg-blue-600 text-white py-2 px-4 rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition duration-200">
                Sign In
            </button>
        </form>
        
        <div class="mt-6 text-center">
            <p class="text-sm text-gray-600">
                Don't have an account? 
                <a href="/auth/register" class="text-blue-600 hover:text-blue-700 font-medium">Create one</a>
            </p>
        </div>
    </div>
</body>
</html>`

		tmpl, err := raymond.Parse(loginTemplate)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		html, err = tmpl.Exec(data)
		if err != nil {
			http.Error(w, "Template execution error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func handleLoginSubmit(w http.ResponseWriter, r *http.Request, fs *lang_adapters.FrameworkServer) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := map[string]any{
		"username": username,
	}

	// Query for user with password_hash
	resultJSON, err := fs.DbExecutor.ExecuteSQL(ctx, "SELECT id, email, password_hash FROM users WHERE email = :username", params, nil)
	if err != nil {
		log.Printf("❌ Database execution failed: %v", err)
		http.Redirect(w, r, "/auth/login?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	var dbResponse struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Error   string           `json:"error"`
		Count   int              `json:"count"`
	}

	if err := json.Unmarshal(resultJSON, &dbResponse); err != nil {
		log.Printf("❌ Failed to parse database response: %v", err)
		http.Redirect(w, r, "/auth/login?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	if !dbResponse.Success {
		log.Printf("❌ Database query failed: %s", dbResponse.Error)
		http.Redirect(w, r, "/auth/login?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	if dbResponse.Count == 0 {
		log.Printf("❌ User not found: %s", username)
		http.Redirect(w, r, "/auth/login?error=Invalid+credentials", http.StatusSeeOther)
		return
	}

	userData := dbResponse.Data[0]

	// Extract email and password hash with safe type assertion
	email, ok := userData["email"].(string)
	if !ok {
		log.Printf("❌ Email field is missing or not a string")
		http.Redirect(w, r, "/auth/login?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	passwordHash, ok := userData["password_hash"].(string)
	if !ok {
		log.Printf("❌ Password hash field is missing or not a string")
		http.Redirect(w, r, "/auth/login?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	id, ok := userData["id"].(float64)
	if !ok {
		http.Redirect(w, r, "/auth/login?error=Internal+Server+Error+ID", http.StatusSeeOther)
		return
	}

	// Validate password using bcrypt
	if !ValidatePassword(password, passwordHash) {
		log.Printf("❌ Invalid password for user: %s", username)
		http.Redirect(w, r, "/auth/login?error=Invalid+credentials", http.StatusSeeOther)
		return
	}

	log.Printf("✅ User authenticated successfully: %s", email)

	user := User{
		Username: email,
		Id:       id,
	}

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"Username": user.Username,
		"Id":       user.Id,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		log.Printf("❌ Failed to create JWT token: %v", err)
		http.Redirect(w, r, "/auth/login?error=Internal+server+error", http.StatusSeeOther)
		return
	}

	// Set JWT as HTTP-only cookie
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    tokenString,
		Path:     "/",
		MaxAge:   24 * 60 * 60, // 24 hours
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	log.Printf("✅ Login successful, redirecting to dashboard")
	// Redirect to dashboard
	http.Redirect(w, r, "/auth/dashboard", http.StatusSeeOther)
}

// handleDashboard renders the protected dashboard page
func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !IsAuthenticated(r) {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	username := getUserFromToken(r)

	dashboardTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dashboard</title>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-100 min-h-screen">
    <nav class="bg-white shadow-sm">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div class="flex justify-between h-16">
                <div class="flex items-center">
                    <h1 class="text-xl font-semibold text-gray-900">Dashboard</h1>
                </div>
                <div class="flex items-center space-x-4">
                    <span class="text-gray-700">Welcome, {{username}}!</span>
                    <form method="POST" action="/auth/logout" class="inline">
                        <button type="submit" 
                                class="bg-red-600 text-white px-4 py-2 rounded-md hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 transition duration-200">
                            Logout
                        </button>
                    </form>
                </div>
            </div>
        </div>
    </nav>

    <main class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
        <div class="px-4 py-6 sm:px-0">
            <div class="border-4 border-dashed border-gray-200 rounded-lg p-8">
                <div class="text-center">
                    <h2 class="text-3xl font-bold text-gray-900 mb-4">Protected Dashboard</h2>
                    <p class="text-gray-600 mb-8">You are successfully logged in as <strong>{{username}}</strong></p>
                    
                    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
                        <div class="bg-blue-50 p-6 rounded-lg">
                            <div class="text-blue-600 text-2xl font-bold">42</div>
                            <div class="text-gray-600">Total Users</div>
                        </div>
                        <div class="bg-green-50 p-6 rounded-lg">
                            <div class="text-green-600 text-2xl font-bold">127</div>
                            <div class="text-gray-600">Active Sessions</div>
                        </div>
                        <div class="bg-purple-50 p-6 rounded-lg">
                            <div class="text-purple-600 text-2xl font-bold">89%</div>
                            <div class="text-gray-600">Uptime</div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </main>
</body>
</html>`

	data := map[string]interface{}{
		"username": username,
	}

	tmpl, err := raymond.Parse(dashboardTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	html, err := tmpl.Exec(data)
	if err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleLogout clears the authentication cookie
func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// isAuthenticated checks if the request has a valid JWT token
func IsAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return false
	}

	token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return false
	}

	return token.Valid
}

// getUserFromToken extracts the username from the JWT token
func getUserFromToken(r *http.Request) string {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return ""
	}

	token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if username, ok := claims["username"].(string); ok {
			return username
		}
	}

	return ""
}

// tryRegisterRoute attempts to register a route, but gracefully handles conflicts
func tryRegisterRoute(mux *http.ServeMux, pattern string, handler func(http.ResponseWriter, *http.Request)) {
	defer func() {
		if r := recover(); r != nil {
			// Handle different panic types that could come from ServeMux
			var errStr string
			switch v := r.(type) {
			case string:
				errStr = v
			case error:
				errStr = v.Error()
			default:
				errStr = fmt.Sprintf("%v", r)
			}
			
			// Check if this is a route conflict panic
			if strings.Contains(errStr, "conflicts with pattern") {
				log.Printf("⚠️ Route %s already registered, skipping manual registration", pattern)
				return
			}
			panic(r) // Re-panic if it's not a route conflict
		}
	}()
	mux.HandleFunc(pattern, handler)
	log.Printf("✅ Manually registered auth route: %s", pattern)
}

func AddLoginRoute(mux *http.ServeMux, fs *lang_adapters.FrameworkServer) {
	// New /auth prefixed routes
	// Note: We defer to manual registration since auth routes need special handling
	tryRegisterRoute(mux, "GET /auth/login", handleLoginPage)
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleLoginSubmit(w, r, fs)
	})
	mux.HandleFunc("GET /auth/register", handleRegisterPage)
	mux.HandleFunc("POST /auth/register", func(w http.ResponseWriter, r *http.Request) {
		handleRegisterSubmit(w, r, fs)
	})
	mux.HandleFunc("GET /auth/dashboard", handleDashboard)
	mux.HandleFunc("POST /auth/logout", handleLogout)

	// Backward compatibility redirects for old URLs
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		// Preserve query parameters (like error messages)
		query := r.URL.RawQuery
		redirectURL := "/auth/login"
		if query != "" {
			redirectURL += "?" + query
		}
		http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
	})
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		// For POST requests, we need to redirect but preserve the form data
		// Since we can't preserve POST data in a redirect, we'll handle the login here
		handleLoginSubmit(w, r, fs)
	})
	mux.HandleFunc("GET /register", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		redirectURL := "/auth/register"
		if query != "" {
			redirectURL += "?" + query
		}
		http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
	})
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		handleRegisterSubmit(w, r, fs)
	})
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/auth/dashboard", http.StatusMovedPermanently)
	})
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		handleLogout(w, r)
	})
}

func handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	if IsAuthenticated(r) {
		http.Redirect(w, r, "/auth/dashboard", http.StatusSeeOther)
		return
	}

	// Get error/success from query params if any
	errorMsg := r.URL.Query().Get("error")
	successMsg := r.URL.Query().Get("success")

	data := map[string]interface{}{}
	if errorMsg != "" {
		data["error"] = errorMsg
	}
	if successMsg != "" {
		data["success"] = successMsg
	}

	// Try to load dynamic template, fallback to hardcoded if needed
	html, err := loadAuthTemplate("register/get.html.hbs", data)
	if err != nil {
		log.Printf("⚠️ Failed to load dynamic register template, using fallback: %v", err)
		// Fallback to hardcoded template
		registerTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Register</title>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-100 min-h-screen flex items-center justify-center">
    <div class="bg-white p-8 rounded-lg shadow-md w-full max-w-md">
        <h2 class="text-2xl font-bold text-center text-gray-800 mb-6">Create Account</h2>
        
        {{#if error}}
        <div class="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded mb-4">
            {{error}}
        </div>
        {{/if}}

        {{#if success}}
        <div class="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded mb-4">
            {{success}}
        </div>
        {{/if}}

        <form method="POST" action="/auth/register" class="space-y-4">
            <div>
                <label for="email" class="block text-sm font-medium text-gray-700 mb-1">Email</label>
                <input type="email" id="email" name="email" required 
                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent">
            </div>
            
            <div>
                <label for="password" class="block text-sm font-medium text-gray-700 mb-1">Password</label>
                <input type="password" id="password" name="password" required minlength="6"
                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent">
                <p class="text-xs text-gray-500 mt-1">Minimum 6 characters</p>
            </div>
            
            <div>
                <label for="confirm_password" class="block text-sm font-medium text-gray-700 mb-1">Confirm Password</label>
                <input type="password" id="confirm_password" name="confirm_password" required 
                       class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent">
            </div>
            
            <button type="submit" 
			class="w-full bg-green-600 text-white py-2 px-4 rounded-md hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500 focus:ring-offset-2 transition duration-200">
                Create Account
            </button>
        </form>
        
        <div class="mt-6 text-center">
            <p class="text-sm text-gray-600">
                Already have an account? 
                <a href="/auth/login" class="text-blue-600 hover:text-blue-700 font-medium">Sign in</a>
            </p>
        </div>
    </div>
</body>
</html>`

		tmpl, err := raymond.Parse(registerTemplate)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		html, err = tmpl.Exec(data)
		if err != nil {
			http.Error(w, "Template execution error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleRegisterSubmit processes the registration form submission
func handleRegisterSubmit(w http.ResponseWriter, r *http.Request, fs *lang_adapters.FrameworkServer) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate form data
	if email == "" || password == "" || confirmPassword == "" {
		http.Redirect(w, r, "/auth/register?error=All+fields+are+required", http.StatusSeeOther)
		return
	}

	if len(password) < 6 {
		http.Redirect(w, r, "/auth/register?error=Password+must+be+at+least+6+characters", http.StatusSeeOther)
		return
	}

	if password != confirmPassword {
		http.Redirect(w, r, "/auth/register?error=Passwords+do+not+match", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if user already exists
	checkParams := map[string]any{
		"email": email,
	}

	checkResultJSON, err := fs.DbExecutor.ExecuteSQL(ctx, "SELECT COUNT(*) as count FROM users WHERE email = :email", checkParams, nil)
	if err != nil {
		log.Printf("❌ Database check failed: %v", err)
		http.Redirect(w, r, "/auth/register?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	var checkResponse struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Error   string           `json:"error"`
		Count   int              `json:"count"`
	}

	if err := json.Unmarshal(checkResultJSON, &checkResponse); err != nil {
		log.Printf("❌ Failed to parse check response: %v", err)
		http.Redirect(w, r, "/auth/register?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	if !checkResponse.Success {
		log.Printf("❌ Database check query failed: %s", checkResponse.Error)
		http.Redirect(w, r, "/auth/register?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	if len(checkResponse.Data) > 0 {
		if count, ok := checkResponse.Data[0]["count"].(float64); ok && count > 0 {
			log.Printf("❌ User already exists: %s", email)
			http.Redirect(w, r, "/auth/register?error=Email+already+registered", http.StatusSeeOther)
			return
		}
	}

	// Hash the password
	hashedPassword, err := HashPassword(password)
	if err != nil {
		log.Printf("❌ Failed to hash password: %v", err)
		http.Redirect(w, r, "/auth/register?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	// Insert new user
	insertParams := map[string]any{
		"email":         email,
		"password_hash": hashedPassword,
	}

	insertResultJSON, err := fs.DbExecutor.ExecuteSQL(ctx, "INSERT INTO users (email, password_hash) VALUES (:email, :password_hash)", insertParams, nil)
	if err != nil {
		log.Printf("❌ Failed to insert user: %v", err)
		http.Redirect(w, r, "/auth/register?error=Failed+to+create+account", http.StatusSeeOther)
		return
	}

	var insertResponse struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}

	if err := json.Unmarshal(insertResultJSON, &insertResponse); err != nil {
		log.Printf("❌ Failed to parse insert response: %v", err)
		http.Redirect(w, r, "/auth/register?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	if !insertResponse.Success {
		log.Printf("❌ Failed to insert user: %s", insertResponse.Error)
		http.Redirect(w, r, "/auth/register?error=Failed+to+create+account", http.StatusSeeOther)
		return
	}

	log.Printf("✅ User registered successfully: %s", email)
	http.Redirect(w, r, "/auth/login?success=Account+created+successfully!+Please+log+in.", http.StatusSeeOther)
}
