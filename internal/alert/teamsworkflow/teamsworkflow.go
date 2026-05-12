package teamsworkflow

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pontostroy/kwatch/internal/config"
	"github.com/pontostroy/kwatch/internal/event"
	"github.com/pontostroy/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

const (
	defaultTeamsTitle = "&#9937; Kwatch detected a crash in pod"
	defaultMaxRetries = 3
	defaultRetryDelay = 5
	adaptiveCardSchema = "http://adaptivecards.io/schemas/adaptive-card.json"
	adaptiveCardVersion = "1.2"
)

var colorStripRed = base64ColorStrip("#FF0000")
var colorStripGreen = base64ColorStrip("#00CC66")

type TeamsWorkflow struct {
	webhook    string
	title      string
	text       string
	maxRetries int
	retryDelay int
	appCfg     *config.App
}

type workflowPayload struct {
	Type        string         `json:"type"`
	Attachments []attachment   `json:"attachments"`
}

type attachment struct {
	ContentType string          `json:"contentType"`
	ContentURL  *string         `json:"contentUrl"`
	Content     json.RawMessage `json:"content"`
}

type adaptiveCard struct {
	Schema          string          `json:"$schema"`
	Type            string          `json:"type"`
	Version         string          `json:"version"`
	Body            []cardElement   `json:"body"`
	MsTeams         msTeamsExt      `json:"msteams"`
	BackgroundImage string          `json:"backgroundImage,omitempty"`
}

type msTeamsExt struct {
	Width string `json:"width"`
}

type cardElement struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	Weight   string      `json:"weight,omitempty"`
	Size     string      `json:"size,omitempty"`
	Color    string      `json:"color,omitempty"`
	Wrap     bool        `json:"wrap,omitempty"`
	IsSubtle bool        `json:"isSubtle,omitempty"`
	Style    string      `json:"style,omitempty"`
	Facts    []fact      `json:"facts,omitempty"`
}

type fact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type simpleMessagePayload struct {
	Type        string         `json:"type"`
	Attachments []simpleAttach `json:"attachments"`
}

type simpleAttach struct {
	ContentType string `json:"contentType"`
	ContentURL  *string `json:"contentUrl"`
	Text        string `json:"text"`
}

func NewTeamsWorkflow(config map[string]interface{}, appCfg *config.App) *TeamsWorkflow {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing TeamsWorkflow with empty flow url")
		return nil
	}

	klog.InfoS("initializing TeamsWorkflow with workflow url", "webhook", webhook)

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	maxRetries, mxOk := config["maxRetries"].(int)
	if !mxOk || maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}

	retryDelay, dlOk := config["retryDelay"].(int)
	if !dlOk || retryDelay == 0 {
		retryDelay = defaultRetryDelay
	}

	return &TeamsWorkflow{
		webhook:    webhook,
		title:      title,
		text:       text,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		appCfg:     appCfg,
	}
}

func (t *TeamsWorkflow) Name() string {
	return "Microsoft Teams Workflow"
}

func (t *TeamsWorkflow) SendEvent(e *event.Event) error {
	payload := t.buildEventPayload(e)
	return t.sendAPI(payload)
}

func (t *TeamsWorkflow) SendMessage(msg string) error {
	payload := t.buildMessagePayload(msg)
	return t.sendAPI(payload)
}

func (t *TeamsWorkflow) sendAPI(payload []byte) error {
	for attempts := 0; attempts < t.maxRetries; attempts++ {
		request, err := http.NewRequest(
			http.MethodPost,
			t.webhook,
			bytes.NewBuffer(payload))
		if err != nil {
			return fmt.Errorf("error creating HTTP request: %v", err)
		}

		request.Header.Set("Content-Type", "application/json")

		client := k8s.GetDefaultClient()
		resp, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("failed to send HTTP request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}

		if resp.StatusCode == http.StatusBadRequest {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("call to workflow returned status %d", resp.StatusCode)
			}
			if strings.Contains(string(body), "TriggerInputSchemaMismatch") {
				return fmt.Errorf(
					"failed to send message due to schema mismatch: %s",
					string(body))
			}
			return fmt.Errorf(
				"call to workflow returned status %d: %s",
				resp.StatusCode,
				string(body))
		}

		if resp.StatusCode == http.StatusAccepted {
			klog.InfoS("Request accepted by workflow",
				"attempt", attempts+1,
				"maxRetries", t.maxRetries)
			return nil
		} else {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf(
					"call to workflow returned status %d", resp.StatusCode)
			}
			return fmt.Errorf(
				"call to workflow returned status %d: %s",
				resp.StatusCode,
				string(body))
		}

		if attempts < t.maxRetries-1 {
			time.Sleep(time.Duration(t.retryDelay) * time.Second)
		}
	}

	return fmt.Errorf("failed to send message after %d attempts", t.maxRetries)
}

func (t *TeamsWorkflow) buildEventPayload(e *event.Event) []byte {
	title := t.title
	if len(title) == 0 {
		title = defaultTeamsTitle
	}

	msg := e.FormatMarkdown(t.appCfg.ClusterName, t.text, "\n")

	body := []cardElement{
		{
			Type:   "TextBlock",
			Text:   title,
			Weight: "Bolder",
			Size:   "Medium",
			Color:  "Attention",
		},
		{
			Type: "TextBlock",
			Text: "",
		},
		{
			Type:  "FactSet",
			Facts: buildEventFacts(e),
		},
		{
			Type:   "TextBlock",
			Text:   "## Events",
			Weight: "Medium",
		},
		{
			Type:     "TextBlock",
			Text:     formatLongText(e.Events),
			Wrap:     true,
			IsSubtle: true,
		},
		{
			Type:   "TextBlock",
			Text:   "## Logs",
			Weight: "Medium",
		},
		{
			Type:     "TextBlock",
			Text:     formatLongText(e.Logs),
			Wrap:     true,
			IsSubtle: true,
		},
		{
			Type:   "TextBlock",
			Text:   msg,
			Wrap:   true,
			IsSubtle: true,
		},
	}

	card := adaptiveCard{
		Schema:     adaptiveCardSchema,
		Type:       "AdaptiveCard",
		Version:    adaptiveCardVersion,
		Body:       body,
		MsTeams:    msTeamsExt{Width: "Full"},
		BackgroundImage: string(colorStripRed),
	}

	cardJSON, _ := json.Marshal(card)

	payload := workflowPayload{
		Type: "message",
		Attachments: []attachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     cardJSON,
			},
		},
	}

	result, _ := json.Marshal(payload)
	return result
}

func (t *TeamsWorkflow) buildMessagePayload(msg string) []byte {
	card := adaptiveCard{
		Schema:     adaptiveCardSchema,
		Type:       "AdaptiveCard",
		Version:    adaptiveCardVersion,
		Body: []cardElement{
			{
				Type:   "TextBlock",
				Text:   msg,
				Wrap:   true,
			},
		},
		MsTeams:         msTeamsExt{Width: "Full"},
		BackgroundImage: string(colorStripGreen),
	}

	cardJSON, _ := json.Marshal(card)

	payload := workflowPayload{
		Type: "message",
		Attachments: []attachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     cardJSON,
			},
		},
	}

	result, _ := json.Marshal(payload)
	return result
}

func buildEventFacts(e *event.Event) []fact {
	return []fact{
		{Title: "Pod", Value: e.PodName},
		{Title: "Container", Value: e.ContainerName},
		{Title: "Namespace", Value: e.Namespace},
		{Title: "Node", Value: e.NodeName},
		{Title: "Reason", Value: e.Reason},
	}
}

func formatLongText(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return "No data available"
	}
	return trimmed
}

func base64ColorStrip(hexColor string) string {
	pixel := createPixel(hexColor)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pixel)
}

func createPixel(hexColor string) []byte {
	r, g, b := hexToRGB(hexColor)

	png := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	for i := 0; i < 16; i++ {
		offset := 44 + i*4
		if offset+3 < len(png) {
			png[offset] = byte(r)
			png[offset+1] = byte(g)
			png[offset+2] = byte(b)
		}
	}

	return png
}

func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 6 {
		r, _ := strconv.ParseInt(hex[0:2], 16, 64)
		g, _ := strconv.ParseInt(hex[2:4], 16, 64)
		b, _ := strconv.ParseInt(hex[4:6], 16, 64)
		return int(r), int(g), int(b)
	}
	return 0, 0, 0
}
