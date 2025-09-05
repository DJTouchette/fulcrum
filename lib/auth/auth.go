package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
}

var jwtSecret = []byte("your-secret-key-change-this-in-production")

var users = map[string]User{
	"admin": {Username: "admin", Password: "password123"},
	"user":  {Username: "user", Password: "userpass"},
}

func AddLoginRoute(mux *http.ServeMux, fs *lang_adapters.FrameworkServer) {
	mux.HandleFunc("GET /login", handleLoginPage)
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		handleLoginSubmit(w, r, fs)
	})
	mux.HandleFunc("GET /dashboard", handleDashboard)
	mux.HandleFunc("POST /logout", handleLogout)
}

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if IsAuthenticated(r) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

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

        <form method="POST" action="/login" class="space-y-4">
            <div>
                <label for="username" class="block text-sm font-medium text-gray-700 mb-1">Username</label>
                <input type="text" id="username" name="username" required 
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
        
        <div class="mt-4 text-sm text-gray-600 text-center">
            <p>Demo credentials:</p>
            <p><strong>admin</strong> / password123</p>
            <p><strong>user</strong> / userpass</p>
        </div>
    </div>
</body>
</html>`

	// Get error from query params if any
	errorMsg := r.URL.Query().Get("error")

	data := map[string]interface{}{}
	if errorMsg != "" {
		data["error"] = errorMsg
	}

	tmpl, err := raymond.Parse(loginTemplate)
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

// handleLoginSubmit processes the login form submission
func handleLoginSubmit(w http.ResponseWriter, r *http.Request, fs *lang_adapters.FrameworkServer) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	params := map[string]any{
		"username": username,
		"password": password,
	}

	resultJSON, err := fs.DbExecutor.ExecuteSQL(ctx, "SELECT * FROM users WHERE email = :username", params, nil)
	if err != nil {
		log.Printf("❌ Database execution failed: %v", err)
		fmt.Errorf("database execution failed: %w", err)
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
		fmt.Errorf("failed to parse database response: %w", err)
		http.Redirect(w, r, "/login?error=Internal+Server+Error", http.StatusSeeOther)
		return
	}

	if !dbResponse.Success {
		log.Printf("❌ Database query failed: %s", dbResponse.Error)
		fmt.Errorf("database query failed: %s", dbResponse.Error)
		http.Redirect(w, r, "/login?error=Not+Found", http.StatusSeeOther)
		return
	}

	if dbResponse.Count == 0 {
		http.Redirect(w, r, "/login?error=Not+Found", http.StatusSeeOther)
		return
	}

	log.Printf("✅ Database query successful: %d records", dbResponse.Count)
	log.Printf("📦 Database response data: %+v", dbResponse.Data)

	user := User{
		Username: dbResponse.Data[0]["email"].(string),
		// Password: password,
	}

	// Validate credentials
	// user, exists := users[username]
	// if !exists || user.Password != password {
	// 	http.Redirect(w, r, "/login?error=Invalid+credentials", http.StatusSeeOther)
	// 	return
	// }

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": user.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		http.Redirect(w, r, "/login?error=Internal+server+error", http.StatusSeeOther)
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

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// handleDashboard renders the protected dashboard page
func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !IsAuthenticated(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
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
                    <form method="POST" action="/logout" class="inline">
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

	http.Redirect(w, r, "/login", http.StatusSeeOther)
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

// Example usage:
// func main() {
// 	mux := http.NewServeMux()
//
// 	// Add the login routes
// 	AddLoginRoute(mux)
//
// 	// Add a root route that redirects to login
// 	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
// 		if isAuthenticated(r) {
// 			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
// 		} else {
// 			http.Redirect(w, r, "/login", http.StatusSeeOther)
// 		}
// 	})
//
// 	fmt.Println("Server starting on :8080")
// 	fmt.Println("Visit http://localhost:8080 to test the login")
// 	http.ListenAndServe(":8080", mux)
// }
