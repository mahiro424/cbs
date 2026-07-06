package httpapi

type Route struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Module    string `json:"module"`
	Operation string `json:"operation"`
}

type Envelope struct {
	Success   bool   `json:"success"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}
