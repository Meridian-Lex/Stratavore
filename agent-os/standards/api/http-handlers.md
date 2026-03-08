# HTTP Handler Patterns

## Route Structure

All routes: `/api/v1/<resource>/<action>`

```
/api/v1/runners/launch
/api/v1/sprints/create
/api/v1/mode/get
```

- No trailing slashes
- Resource noun, then action verb
- Registered via `mux.HandleFunc` in `NewHTTPServer`

## Handler Shape

Every handler follows this exact structure:

```go
func (s *HTTPServer) handleCreateFoo(w http.ResponseWriter, r *http.Request) {
    // 1. Method check
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // 2. Decode request
    var req api.CreateFooRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // 3. Delegate to handler/service
    resp, err := s.handler.CreateFoo(r.Context(), &req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // 4. Respond
    s.respondJSON(w, resp)
}
```

## Response Helper

All JSON responses via `respondJSON` — never inline:

```go
func (s *HTTPServer) respondJSON(w http.ResponseWriter, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}
```

## Error Responses

Errors embedded in response struct (not HTTP error codes) for soft errors:

```go
s.respondJSON(w, &api.GetModeResponse{Error: err.Error()})
```

Hard errors (bad request, method not allowed): `http.Error()` with appropriate status code.

## Request/Response Types

All request and response types defined in `pkg/api/` as plain Go structs.
Never inline anonymous structs in handlers.
