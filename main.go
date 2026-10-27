package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"mini-shop-backend/auth"
	"mini-shop-backend/database"
	"mini-shop-backend/handlers"
	"mini-shop-backend/repository"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; using environment variables")
	}

	db, err := database.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Initialize repositories and handlers
	userRepo := repository.NewUserRepository(db)
	authHandler := handlers.NewAuthHandler(userRepo)

	shopRepo := repository.NewShopRepository(db)
	shopHandler := handlers.NewShopHandler(shopRepo)

	productRepo := repository.NewProductRepository(db)
	productHandler := handlers.NewProductHandler(productRepo, shopRepo)
	productHandler.UploadDir = "uploads"
	orderRepo := repository.NewOrderRepository(db)
	orderHandler := handlers.NewOrderHandler(orderRepo)

	mux := http.NewServeMux()

	// Authentication routes
	mux.HandleFunc("POST /auth/register", authHandler.Register)
	mux.HandleFunc("POST /auth/verify-otp", authHandler.VerifyOTP)
	mux.Handle("POST /orders", auth.AuthMiddleware(auth.RequireRole("customer", http.HandlerFunc(orderHandler.Create))))
	mux.Handle("GET /my/orders", auth.AuthMiddleware(auth.RequireRole("customer", http.HandlerFunc(orderHandler.ListForCustomer))))
	mux.Handle("GET /seller/orders", auth.AuthMiddleware(auth.RequireRole("seller", http.HandlerFunc(orderHandler.ListForSeller))))
	mux.Handle("PUT /orders/{id}/status", auth.AuthMiddleware(auth.RequireRole("seller", http.HandlerFunc(orderHandler.UpdateStatus))))

	// Get the currently authenticated user
	mux.Handle("GET /auth/me", auth.AuthMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)

			user, err := userRepo.FindByID(r.Context(), claims.UserID)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "User account not found"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(user)
		}),
	))

	// Shop routes
	mux.Handle(
		"POST /shops",
		auth.AuthMiddleware(
			auth.RequireRole(
				"seller",
				http.HandlerFunc(shopHandler.CreateShop),
			),
		),
	)
	mux.Handle("PUT /shops/{id}", auth.AuthMiddleware(auth.RequireRole("seller", http.HandlerFunc(shopHandler.UpdateShop))))

	mux.Handle("GET /shops", optionalAuth(http.HandlerFunc(shopHandler.GetShops)))

	// Product routes
	mux.Handle(
		"POST /shops/{id}/products",
		auth.AuthMiddleware(
			auth.RequireRole(
				"seller",
				http.HandlerFunc(productHandler.CreateProduct),
			),
		),
	)
	mux.Handle("PUT /products/{id}", auth.AuthMiddleware(auth.RequireRole("seller", http.HandlerFunc(productHandler.UpdateProduct))))
	mux.Handle("DELETE /products/{id}", auth.AuthMiddleware(auth.RequireRole("seller", http.HandlerFunc(productHandler.DeleteProduct))))
	mux.Handle("POST /products/{id}/image", auth.AuthMiddleware(auth.RequireRole("seller", http.HandlerFunc(productHandler.UploadProductImage))))

	mux.HandleFunc(
		"GET /shops/{id}/products",
		productHandler.GetProductsByShop,
	)

	mux.Handle(
		"DELETE /shops/{id}",
		auth.AuthMiddleware(
			auth.RequireRole(
				"seller",
				http.HandlerFunc(shopHandler.DeleteShop),
			),
		),
	)

	log.Println("Connected to mini_shop_v2 successfully!")
	log.Println("Server running at http://localhost:8080")
	if err := os.MkdirAll("uploads", 0755); err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	if err := http.ListenAndServe(":8080", corsMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			next.ServeHTTP(w, r)
			return
		}
		auth.AuthMiddleware(next).ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:5173": true,
		"http://127.0.0.1:5173": true,
		"http://mini-shop-frontend-roshan-2026.s3-website.ap-south-1.amazonaws.com": true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}

		if r.Method == http.MethodOptions && allowedOrigins[origin] {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
