package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"mini-shop-backend/auth"
	"mini-shop-backend/repository"
)

type OrderHandler struct{ Orders *repository.OrderRepository }

func NewOrderHandler(orders *repository.OrderRepository) *OrderHandler {
	return &OrderHandler{Orders: orders}
}

type createOrderRequest struct {
	Items []repository.OrderLineInput `json:"items"`
}

func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil || claims.Role != "customer" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "A customer account is required to place an order"})
		return
	}
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	ids, err := h.Orders.CreateForCustomer(r.Context(), claims.UserID, req.Items)
	if err != nil {
		if errors.Is(err, repository.ErrOrderStock) || strings.Contains(err.Error(), "invalid product") || strings.Contains(err.Error(), "cart must contain") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not place order"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"order_ids": ids, "message": "Order placed and sent to the seller"})
}

func (h *OrderHandler) ListForSeller(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil || claims.Role != "seller" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Seller account required"})
		return
	}
	orders, err := h.Orders.ListForSeller(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not retrieve orders"})
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (h *OrderHandler) ListForCustomer(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil || claims.Role != "customer" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Customer account required"})
		return
	}
	orders, err := h.Orders.ListForCustomer(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not retrieve your orders"})
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

type orderStatusRequest struct {
	Status string `json:"status"`
}

func (h *OrderHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil || claims.Role != "seller" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Seller account required"})
		return
	}
	orderID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orderID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid order ID"})
		return
	}
	var req orderStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Status != "accepted" && req.Status != "rejected") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Status must be accepted or rejected"})
		return
	}
	updated, err := h.Orders.SetStatusByOwner(r.Context(), orderID, claims.UserID, req.Status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update order"})
		return
	}
	if !updated {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Pending order not found for your shop"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}
