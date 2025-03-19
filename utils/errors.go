package utils

// CustomError digunakan untuk error dengan status code yang spesifik
type CustomError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
}

func (e *CustomError) Error() string {
	return e.Message
}

// NewCustomError Fungsi helper untuk membuat CustomError
func NewCustomError(statusCode int, message string) *CustomError {
	return &CustomError{StatusCode: statusCode, Message: message}
}
