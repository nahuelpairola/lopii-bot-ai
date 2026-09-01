package orchestrator

import "testing"

func TestIsToolUseFailed_RecognizesGroqRefusal(t *testing.T) {
	body := []byte(`{"error":{"message":"Tool choice is required, but model did not call a tool","type":"invalid_request_error","code":"tool_use_failed","failed_generation":""}}`)
	if !isToolUseFailed(body) {
		t.Error("no reconoció el tool_use_failed de Groq")
	}
}

func TestIsToolUseFailed_OtherErrorIsNotIt(t *testing.T) {
	body := []byte(`{"error":{"message":"Invalid API Key","type":"invalid_request_error","code":"invalid_api_key"}}`)
	if isToolUseFailed(body) {
		t.Error("confundió otro error 400 con tool_use_failed")
	}
}

func TestIsToolUseFailed_MentionInMessageIsNotEnough(t *testing.T) {
	body := []byte(`{"error":{"message":"something about tool_use_failed happened","code":"server_error"}}`)
	if isToolUseFailed(body) {
		t.Error("se dejó engañar por la mención en el mensaje")
	}
}

func TestIsToolUseFailed_GarbageBodyIsSafe(t *testing.T) {
	if isToolUseFailed([]byte("no soy json")) {
		t.Error("un body no-JSON no puede dar true")
	}
}
