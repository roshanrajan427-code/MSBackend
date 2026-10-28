package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mini-shop-backend/auth"
	"mini-shop-backend/repository"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ProductHandler struct {
	Products  *repository.ProductRepository
	Shops     *repository.ShopRepository
	UploadDir string
	S3Client  *s3.Client
	S3Bucket  string
}

func NewProductHandler(
	products *repository.ProductRepository,
	shops *repository.ShopRepository,
) *ProductHandler {
	return &ProductHandler{
		Products: products,
		Shops:    shops,
	}
}
func (h *ProductHandler) presignProductImage(
	ctx context.Context,
	objectKey string,
) (string, error) {
	if objectKey == "" {
		return "", nil
	}

	presigner := s3.NewPresignClient(h.S3Client)

	result, err := presigner.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(h.S3Bucket),
			Key:    aws.String(objectKey),
		},
		func(opts *s3.PresignOptions) {
			opts.Expires = 15 * time.Minute
		},
	)
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

type productRequest struct {
	ShopID int64    `json:"shop_id"`
	Name   string   `json:"name"`
	Stock  int      `json:"stock"`
	Price  *float64 `json:"price"`
	Color  string   `json:"color"`
	Size   string   `json:"size"`
}

func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req productRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		message := "Invalid JSON"

		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			switch typeErr.Field {
			case "price":
				message = "Price must be a valid number"
			case "stock":
				message = "Stock must be a whole number"
			}
		}

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": message,
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)

	req.Color, req.Size = strings.TrimSpace(req.Color), strings.TrimSpace(req.Size)
	if req.ShopID <= 0 || validateProduct(req) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid product details. Check the name, stock, price, colour, and size.",
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

	belongs, err := h.Shops.BelongsToOwner(
		r.Context(),
		req.ShopID,
		claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not verify shop ownership",
		})
		return
	}

	if !belongs {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "You cannot add products to this shop",
		})
		return
	}
	product, err := h.Products.Create(
		r.Context(),
		req.ShopID,
		req.Name,
		req.Stock,
		*req.Price,
		req.Color,
		req.Size,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create product",
		})
		return
	}

	writeJSON(w, http.StatusCreated, product)
}

func (h *ProductHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || productID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}
	var req productRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if err := validateProduct(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	product, updated, err := h.Products.UpdateByOwner(r.Context(), productID, claims.UserID, strings.TrimSpace(req.Name), req.Stock, *req.Price, strings.TrimSpace(req.Color), strings.TrimSpace(req.Size))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update product"})
		return
	}
	if !updated {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found or you do not own its shop"})
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *ProductHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || productID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	deleted, err := h.Products.DeleteByOwner(r.Context(), productID, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not delete product"})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found or you do not own its shop"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Product deleted successfully"})
}

func validateProduct(req productRequest) error {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 40 {
		return fmt.Errorf("Product name is required and must be at most 40 characters")
	}
	if req.Stock < 0 {
		return fmt.Errorf("Stock must be a whole number greater than or equal to 0")
	}
	if req.Price == nil {
		return fmt.Errorf("Price is required")
	}
	if strings.ContainsAny(req.Name, "<>") {
		return fmt.Errorf("Product name cannot contain HTML tags")
	}
	if *req.Price < 0 || *req.Price > 1000000 {
		return fmt.Errorf("Price must be greater than or equal to 0 and less than or equal to 1000000")
	}
	if len(strings.TrimSpace(req.Color)) > 30 || len(strings.TrimSpace(req.Size)) > 30 {
		return fmt.Errorf("Colour and size must be at most 30 characters")
	}
	return nil
}

func (h *ProductHandler) UploadProductImage(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || productID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}
	claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Choose an image up to 5 MB"})
		return
	}
	defer file.Close()
	if err := h.saveProductImage(w, r, file, header, productID, claims.UserID); err != nil {
		status := http.StatusBadRequest
		if strings.HasPrefix(err.Error(), "database:") ||
			strings.HasPrefix(err.Error(), "storage:") {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, map[string]string{"error": strings.TrimPrefix(err.Error(), "database:")})
		return
	}
}

func (h *ProductHandler) saveProductImage(
	w http.ResponseWriter,
	r *http.Request,
	file multipart.File,
	header *multipart.FileHeader,
	productID, ownerID int64,
) error {
	if header.Size > 5<<20 {
		return fmt.Errorf("Image must be 5 MB or smaller")
	}

	data, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil || len(data) > 5<<20 {
		return fmt.Errorf("Image must be 5 MB or smaller")
	}

	mimeType := http.DetectContentType(data)
	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
		"image/gif":  ".gif",
	}

	extension, valid := extensions[mimeType]
	if !valid {
		return fmt.Errorf("Use a JPEG, PNG, WebP, or GIF image")
	}

	if h.S3Client == nil || h.S3Bucket == "" {
		return fmt.Errorf("storage: S3 is not configured")
	}
	owns, err := h.Products.OwnsProduct(
		r.Context(),
		productID,
		ownerID,
	)
	if err != nil {
		return fmt.Errorf("database: Could not verify product ownership")
	}
	if !owns {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Product not found or you do not own its shop",
		})
		return nil
	}

	randomName := make([]byte, 16)
	if _, err := rand.Read(randomName); err != nil {
		return fmt.Errorf("Could not generate image filename")
	}

	filename := hex.EncodeToString(randomName) + extension
	objectKey := "products/" + filename

	_, err = h.S3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &h.S3Bucket,
		Key:         &objectKey,
		Body:        bytes.NewReader(data),
		ContentType: &mimeType,
	})
	if err != nil {
		return fmt.Errorf("storage: Could not upload image to S3")
	}

	// Store the S3 object key in PostgreSQL.
	product, updated, err := h.Products.SetImageByOwner(
		r.Context(),
		productID,
		ownerID,
		objectKey,
	)
	if err != nil {
		// The S3 upload succeeded, but the database update failed.
		return fmt.Errorf("database: Could not update image")
	}

	if !updated {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Product not found or you do not own its shop",
		})
		return nil
	}

	writeJSON(w, http.StatusOK, product)
	return nil
}

func (h *ProductHandler) GetProductsByShop(
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

	products, err := h.Products.GetByShopID(r.Context(), shopID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not retrieve products",
		})
		return
	}

	for i := range products {
		if products[i].ImageURL == "" {
			continue
		}

		// Existing local-upload paths remain unchanged.
		if strings.HasPrefix(products[i].ImageURL, "/uploads/") {
			continue
		}

		imageURL, err := h.presignProductImage(
			r.Context(),
			products[i].ImageURL,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Could not generate product image URL",
			})
			return
		}

		products[i].ImageURL = imageURL
	}

	writeJSON(w, http.StatusOK, products)
}
