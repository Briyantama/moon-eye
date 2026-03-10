package httpx

// ErrorBody is the standard error envelope returned by the API.
type ErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
