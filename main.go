package main

import (
	"HalalMate/config/database"
	"HalalMate/middleware"
	v1 "HalalMate/routes/v1"
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	// "github.com/joho/godotenv"

	"github.com/gin-gonic/gin"
)

func main() {

	//firebase init
	database.InitFirebase()

	// Load environment variables

	// err := godotenv.Load()
	// if err != nil {
	// 	log.Println("⚠️  No .env file found, using default values")
	// }

	// Setup Gin router
	r := gin.Default()

	// Pasang middleware error handler
	r.Use(middleware.ErrorHandlerMiddleware())

	// CORS Middleware
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // Allow all origins
		AllowMethods:     []string{"GET"},
		AllowHeaders:     []string{"Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Register all routes
	v1.RegisterRoutes(r)

	log.Println("🚀 Server running on http://localhost:8080")
	r.Run(":8080")

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port", port)
	r.Run(":" + port)
}
