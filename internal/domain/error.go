package domain

type ErrorResponse struct {
	Status string `json:"status" example:"error"`
	Error  string `json:"error" example:"Detailed error message here"`
}
