package main

import (
	"HalalMate/config/database"
	"HalalMate/middleware"
	v1 "HalalMate/routes/v1"
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/joho/godotenv"

	"github.com/gin-gonic/gin"
)

func main() {

	// Load environment variables

	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  No .env file found, using default values")
	}

	//firebase init
	database.InitFirebase()

	// Setup Gin router
	r := gin.Default()

	// Disable automatic redirects to prevent 307 issues
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// Debug middleware to log requests
	r.Use(func(c *gin.Context) {
		log.Printf("Incoming request: %s %s from %s", c.Request.Method, c.Request.URL.Path, c.Request.RemoteAddr)
		log.Printf("Headers: %v", c.Request.Header)
		c.Next()
	})

	// Pasang middleware error handler
	r.Use(middleware.ErrorHandlerMiddleware())

	// CORS Middleware
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type", "Cache-Control", "Connection"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// Register all routes
	v1.RegisterRoutes(r)

	// Add global OPTIONS handler to prevent redirects
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(204)
	})

	log.Println("🚀 Server running on http://localhost:8080")
	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port", port)
	r.Run(":" + port)
}
