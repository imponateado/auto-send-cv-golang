package domain

// ErrorResponse represents the JSON response structure when an error occurs.
type ErrorResponse struct {
	Status string `json:"status" example:"error"`
	Error  string `json:"error" example:"Detailed error message here"`
}
