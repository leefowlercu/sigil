package steps

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

type mockGatewayResponse struct {
	statusCode int
	body       any
}

type openRouterMockServer struct {
	mu            sync.Mutex
	server        *httptest.Server
	requestBodies []map[string]any
	requestPaths  []string
	responses     []mockGatewayResponse
}

func newOpenRouterMockServer() *openRouterMockServer {
	mockServer := &openRouterMockServer{
		requestBodies: make([]map[string]any, 0, 4),
		requestPaths:  make([]string, 0, 4),
		responses:     make([]mockGatewayResponse, 0, 4),
	}

	mockServer.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)

		mockServer.mu.Lock()
		mockServer.requestBodies = append(mockServer.requestBodies, cloneAnyMap(body))
		mockServer.requestPaths = append(mockServer.requestPaths, request.URL.Path)
		responseIndex := len(mockServer.requestBodies) - 1
		response := mockServer.responseForIndex(responseIndex)
		mockServer.mu.Unlock()

		statusCode := response.statusCode
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(statusCode)

		responseBody := response.body
		if contextRef := extractContextRefFromRequest(body); contextRef != "" {
			responseBody = replaceContextRefPlaceholders(responseBody, contextRef)
		}

		switch typedBody := responseBody.(type) {
		case nil:
			_, _ = writer.Write([]byte(`{"error":"mock response not configured"}`))
		case string:
			_, _ = writer.Write([]byte(typedBody))
		default:
			encoded, err := json.Marshal(typedBody)
			if err != nil {
				_, _ = writer.Write([]byte(`{"error":"mock response marshal failure"}`))
				return
			}
			_, _ = writer.Write(encoded)
		}
	}))

	return mockServer
}

func (server *openRouterMockServer) URL() string {
	if server == nil || server.server == nil {
		return ""
	}

	return server.server.URL
}

func (server *openRouterMockServer) SetResponses(responses ...mockGatewayResponse) {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.responses = append([]mockGatewayResponse{}, responses...)
}

func (server *openRouterMockServer) RequestCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return len(server.requestBodies)
}

func (server *openRouterMockServer) LastRequestBody() map[string]any {
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.requestBodies) == 0 {
		return nil
	}

	return cloneAnyMap(server.requestBodies[len(server.requestBodies)-1])
}

func (server *openRouterMockServer) LastRequestPath() string {
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.requestPaths) == 0 {
		return ""
	}

	return server.requestPaths[len(server.requestPaths)-1]
}

func (server *openRouterMockServer) Close() {
	if server == nil || server.server == nil {
		return
	}

	server.server.Close()
}

func (server *openRouterMockServer) responseForIndex(index int) mockGatewayResponse {
	if len(server.responses) == 0 {
		return mockGatewayResponse{}
	}
	if index < 0 {
		index = 0
	}
	if index >= len(server.responses) {
		index = len(server.responses) - 1
	}

	return server.responses[index]
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}

	return cloned
}

func extractContextRefFromRequest(requestBody map[string]any) string {
	input, ok := requestBody["input"].([]any)
	if !ok {
		return ""
	}

	for _, item := range input {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		if role != "user" {
			continue
		}

		contentRef := extractContextRefFromMessageContent(message["content"])
		if strings.TrimSpace(contentRef) != "" {
			return contentRef
		}
	}

	return ""
}

func extractContextRefFromMessageContent(content any) string {
	switch typed := content.(type) {
	case string:
		return extractContextRefFromJSONText(typed)
	case []any:
		for _, block := range typed {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			text, _ := blockMap["text"].(string)
			if ref := extractContextRefFromJSONText(text); strings.TrimSpace(ref) != "" {
				return ref
			}
		}
	}

	return ""
}

func extractContextRefFromJSONText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	envelope := make(map[string]any)
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return ""
	}

	contextMetadata, ok := envelope["context_metadata"].(map[string]any)
	if !ok {
		return ""
	}

	ref, _ := contextMetadata["context_ref"].(string)
	return strings.TrimSpace(ref)
}

func replaceContextRefPlaceholders(value any, contextRef string) any {
	switch typed := value.(type) {
	case string:
		return strings.ReplaceAll(typed, "__context_ref__", contextRef)
	case []any:
		replaced := make([]any, 0, len(typed))
		for _, item := range typed {
			replaced = append(replaced, replaceContextRefPlaceholders(item, contextRef))
		}
		return replaced
	case map[string]any:
		replaced := make(map[string]any, len(typed))
		for key, item := range typed {
			replaced[key] = replaceContextRefPlaceholders(item, contextRef)
		}
		return replaced
	default:
		return value
	}
}
