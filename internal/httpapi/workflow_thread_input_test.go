package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"lumi/internal/agent"
)

func TestWorkflowThreadReadOnlyErrorEnvelope(t *testing.T) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), recorder)
	ErrorHandler(agentAPIError(&agent.Error{Code: agent.CodeWorkflowThreadReadOnly, Message: "工作流线程不接受聊天输入", Details: "请新建对话继续。"}), ctx)
	var response Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || response.Success || response.Data != nil || response.Error == nil || response.Error.Code != agent.CodeWorkflowThreadReadOnly {
		t.Fatalf("status=%d response=%+v", recorder.Code, response)
	}
}
