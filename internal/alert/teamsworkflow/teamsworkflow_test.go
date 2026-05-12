package teamsworkflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pontostroy/kwatch/internal/config"
	"github.com/pontostroy/kwatch/internal/event"
	"github.com/stretchr/testify/assert"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)
	c := NewTeamsWorkflow(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestNewTeamsWorkflow(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(t, c)
	assert.Equal(t, "http://example.com", c.webhook)
	assert.Equal(t, "Microsoft Teams Workflow", c.Name())
}

func TestNewTeamsWorkflowWithCustomSettings(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook":    "http://example.com",
		"title":      "Custom Title",
		"text":       "Custom Text",
		"maxRetries": 5,
		"retryDelay": 10,
	}
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(t, c)
	assert.Equal(t, "http://example.com", c.webhook)
	assert.Equal(t, "Custom Title", c.title)
	assert.Equal(t, "Custom Text", c.text)
	assert.Equal(t, 5, c.maxRetries)
	assert.Equal(t, 10, c.retryDelay)
}

func TestSendEvent(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	e := &event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "test-namespace",
		NodeName:      "test-node",
		Reason:        "CrashLoopBackOff",
		Logs:          "test-logs",
		Events:        "test-events",
	}

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendEvent(e)
	assert.NoError(t, err)
}

func TestSendMessage(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://localhost",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendMessage("test message")
	assert.NoError(t, err)
}

func TestSendMessageErrorSchemaMismatch(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`TriggerInputSchemaMismatch`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(c)
	assert.Error(c.SendMessage("test"))
}

func TestSendMessageErrorBadRequest(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(c)
	assert.Error(c.SendMessage("test"))
}

func TestSendMessageErrorServer(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(c)
	assert.Error(c.SendMessage("test"))
}

func TestSendMessageAccepted(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"webhook":    "http://example.com",
		"maxRetries": 3,
		"retryDelay": 1,
	}
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(c)
	c.webhook = s.URL
	assert.NoError(c.SendMessage("test"))
}

func TestSendAPI(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	configMap := map[string]interface{}{
		"webhook": server.URL,
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	payload := []byte(`{"type":"message","attachments":[]}`)
	err := teams.sendAPI(payload)
	assert.NoError(t, err)
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	appCfg := &config.App{ClusterName: "dev"}

	configMap := map[string]interface{}{
		"webhook": "h ttp://localhost/%s",
	}
	c := NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(c)
	assert.Error(c.SendMessage("test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}
	c = NewTeamsWorkflow(configMap, appCfg)
	assert.NotNil(c)
	assert.Error(c.SendMessage("test"))
}

func TestBuildRequestBodyEvent(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Test Title",
		"text":    "Test Text",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	e := &event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "test-namespace",
		NodeName:      "test-node",
		Reason:        "CrashLoopBackOff",
		Logs:          "test-logs",
		Events:        "test-events",
	}

	payload := teams.buildEventPayload(e)
	var result workflowPayload
	err := json.Unmarshal(payload, &result)
	assert.NoError(t, err)
	assert.Equal(t, "message", result.Type)
	assert.Len(t, result.Attachments, 1)

	var card adaptiveCard
	err = json.Unmarshal(result.Attachments[0].Content, &card)
	assert.NoError(t, err)
	assert.Equal(t, adaptiveCardSchema, card.Schema)
	assert.Equal(t, "AdaptiveCard", card.Type)
	assert.Equal(t, adaptiveCardVersion, card.Version)
	assert.Equal(t, "Full", card.MsTeams.Width)
	assert.Contains(t, card.BackgroundImage, "data:image/png;base64")
	assert.NotEmpty(t, card.Body)

	// Check first element is TextBlock with title
	assert.Equal(t, "TextBlock", card.Body[0].Type)
	assert.Equal(t, "Attention", card.Body[0].Color)
	assert.Equal(t, "Bolder", card.Body[0].Weight)

	// Check FactSet
	assert.Equal(t, "FactSet", card.Body[2].Type)
	assert.NotEmpty(t, card.Body[2].Facts)

	// Check facts contain expected data
	factMap := make(map[string]string)
	for _, f := range card.Body[2].Facts {
		factMap[f.Title] = f.Value
	}
	assert.Equal(t, "test-pod", factMap["Pod"])
	assert.Equal(t, "test-container", factMap["Container"])
	assert.Equal(t, "test-namespace", factMap["Namespace"])
	assert.Equal(t, "test-node", factMap["Node"])
	assert.Equal(t, "CrashLoopBackOff", factMap["Reason"])
}

func TestBuildRequestBodyEventDefaultTitle(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	e := &event.Event{
		PodName:   "test-pod",
		Namespace: "test-namespace",
		Reason:    "CrashLoopBackOff",
		Logs:      "test-logs",
		Events:    "test-events",
	}

	payload := teams.buildEventPayload(e)
	var result workflowPayload
	err := json.Unmarshal(payload, &result)
	assert.NoError(t, err)

	var card adaptiveCard
	err = json.Unmarshal(result.Attachments[0].Content, &card)
	assert.NoError(t, err)
	assert.Contains(t, card.Body[0].Text, "Kwatch")
}

func TestBuildRequestBodyMessage(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	payload := teams.buildMessagePayload("test message")
	var result workflowPayload
	err := json.Unmarshal(payload, &result)
	assert.NoError(t, err)
	assert.Equal(t, "message", result.Type)
	assert.Len(t, result.Attachments, 1)

	var card adaptiveCard
	err = json.Unmarshal(result.Attachments[0].Content, &card)
	assert.NoError(t, err)
	assert.Equal(t, adaptiveCardSchema, card.Schema)
	assert.Equal(t, "AdaptiveCard", card.Type)
	assert.Equal(t, "Full", card.MsTeams.Width)
	assert.Contains(t, card.BackgroundImage, "data:image/png;base64")
	assert.Equal(t, "test message", card.Body[0].Text)
}

func TestBuildRequestBodyEventWithEmptyLogs(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	e := &event.Event{
		PodName:   "test-pod",
		Namespace: "test-namespace",
		Reason:    "CrashLoopBackOff",
		Logs:      "",
		Events:    "",
	}

	payload := teams.buildEventPayload(e)
	var result workflowPayload
	err := json.Unmarshal(payload, &result)
	assert.NoError(t, err)

	var card adaptiveCard
	err = json.Unmarshal(result.Attachments[0].Content, &card)
	assert.NoError(t, err)

	// Check that "No data available" is shown for empty events/logs
	hasNoData := false
	for _, elem := range card.Body {
		if elem.Text == "No data available" {
			hasNoData = true
			break
		}
	}
	assert.True(t, hasNoData)
}

func TestHexToRGB(t *testing.T) {
	r, g, b := hexToRGB("#FF0000")
	assert.Equal(t, 255, r)
	assert.Equal(t, 0, g)
	assert.Equal(t, 0, b)

	r, g, b = hexToRGB("#00CC66")
	assert.Equal(t, 0, r)
	assert.Equal(t, 204, g)
	assert.Equal(t, 102, b)

	r, g, b = hexToRGB("invalid")
	assert.Equal(t, 0, r)
	assert.Equal(t, 0, g)
	assert.Equal(t, 0, b)
}

func TestBase64ColorStrip(t *testing.T) {
	result := base64ColorStrip("#FF0000")
	assert.Contains(t, result, "data:image/png;base64")
}

func TestFormatLongText(t *testing.T) {
	assert.Equal(t, "some text", formatLongText("some text"))
	assert.Equal(t, "some text", formatLongText("  some text  "))
	assert.Equal(t, "No data available", formatLongText(""))
	assert.Equal(t, "No data available", formatLongText("   "))
}

func TestSendEventWithCustomTitle(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Custom Title",
		"text":    "Custom Text",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	e := &event.Event{
		PodName:   "test-pod",
		Namespace: "test-namespace",
		Reason:    "CrashLoopBackOff",
		Logs:      "test-logs",
		Events:    "test-events",
	}

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendEvent(e)
	assert.NoError(t, err)
}

func TestSendMessageBadRequestWithBody(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeamsWorkflow(configMap, appCfg)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "bad request details"}`))
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendMessage("test message")
	assert.Error(t, err)
}
