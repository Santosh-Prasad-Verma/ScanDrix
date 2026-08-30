package razorpay

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RazorpayClient communicates directly with Razorpay REST APIs.
type RazorpayClient struct {
	keyID      string
	keySecret  string
	baseURL    string
	httpClient *http.Client
}

// NewRazorpayClient initializes a client with API credentials.
func NewRazorpayClient(keyID, keySecret string) *RazorpayClient {
	return &RazorpayClient{
		keyID:     keyID,
		keySecret: keySecret,
		baseURL:   "https://api.razorpay.com/v1",
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SetBaseURL allows overriding the API endpoint (for unit test mock servers).
func (c *RazorpayClient) SetBaseURL(url string) {
	c.baseURL = url
}

// OrderRequest contains fields required to initialize a Razorpay checkout order.
type OrderRequest struct {
	Amount   int64             `json:"amount"` // In smallest currency unit (e.g. paise: 100 paise = 1 INR)
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Notes    map[string]string `json:"notes,omitempty"`
}

// OrderResponse represents a created Razorpay order.
type OrderResponse struct {
	ID        string            `json:"id"`
	Entity    string            `json:"entity"`
	Amount    int64             `json:"amount"`
	AmountDue int64             `json:"amount_due"`
	Currency  string            `json:"currency"`
	Receipt   string            `json:"receipt"`
	Status    string            `json:"status"`
	Attempts  int               `json:"attempts"`
	Notes     map[string]string `json:"notes"`
	CreatedAt int64             `json:"created_at"`
}

// CreateOrder registers a new order with Razorpay.
func (c *RazorpayClient) CreateOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	if c.keyID == "" || c.keySecret == "" {
		return nil, errors.New("razorpay credentials not configured (RAZORPAY_KEY_ID or RAZORPAY_KEY_SECRET missing)")
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling order payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/orders", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating HTTP request: %w", err)
	}

	// Basic Auth credentials
	authStr := base64.StdEncoding.EncodeToString([]byte(c.keyID + ":" + c.keySecret))
	httpReq.Header.Set("Authorization", "Basic "+authStr)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("razorpay request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading razorpay response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("razorpay order creation failed (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var orderResp OrderResponse
	if err := json.Unmarshal(respBytes, &orderResp); err != nil {
		return nil, fmt.Errorf("failed unmarshaling razorpay response: %w", err)
	}

	return &orderResp, nil
}
