package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"mini-shop-backend/auth"
	"mini-shop-backend/models"
	"mini-shop-backend/repository"
)

type ShopHandler struct {
	Shops *repository.ShopRepository
}

func NewShopHandler(shops *repository.ShopRepository) *ShopHandler {
	return &ShopHandler{Shops: shops}
}

type createShopRequest struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Location string `json:"location"`
}

func (h *ShopHandler) CreateShop(w http.ResponseWriter, r *http.Request) {
	var req createShopRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON",
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.Location = strings.TrimSpace(req.Location)

	if req.Name == "" || len(req.Name) > 50 ||
		req.Category == "" || len(req.Category) > 20 ||
		req.Location == "" || len(req.Location) > 30 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid shop details",
		})
		return
	}

	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	shop, err := h.Shops.Create(
		r.Context(),
		claims.UserID,
		req.Name,
		req.Category,
		req.Location,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "A shop with these details already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create shop",
		})
		return
	}

	writeJSON(w, http.StatusCreated, shop)
}

func (h *ShopHandler) UpdateShop(w http.ResponseWriter, r *http.Request) {
	shopID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || shopID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid shop ID"})
		return
	}
	var req createShopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	req.Name, req.Category, req.Location = strings.TrimSpace(req.Name), strings.TrimSpace(req.Category), strings.TrimSpace(req.Location)
	if req.Name == "" || len(req.Name) > 50 || req.Category == "" || len(req.Category) > 20 || req.Location == "" || len(req.Location) > 30 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Shop name (1–50), category (1–20), and location (1–30) are required"})
		return
	}
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	shop, updated, err := h.Shops.UpdateByOwner(r.Context(), shopID, claims.UserID, req.Name, req.Category, req.Location)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update shop"})
		return
	}
	if !updated {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Shop not found or you do not own this shop"})
		return
	}
	writeJSON(w, http.StatusOK, shop)
}
func (h *ShopHandler) GetShops(w http.ResponseWriter, r *http.Request) {
	var shops []models.Shop
	var err error
	if claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims); ok && claims.Role == "seller" {
		shops, err = h.Shops.GetByOwner(r.Context(), claims.UserID)
	} else {
		shops, err = h.Shops.GetAll(r.Context())
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not retrieve shops",
		})
		return
	}

	writeJSON(w, http.StatusOK, shops)
}
func (h *ShopHandler) DeleteShop(
	w http.ResponseWriter,
	r *http.Request,
) {
	shopID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || shopID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid shop ID",
		})
		return
	}

	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	deleted, err := h.Shops.DeleteByOwner(
		r.Context(),
		shopID,
		claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not delete shop",
		})
		return
	}

	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Shop not found or you do not own this shop",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Shop deleted successfully",
	})
}
