package notify

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"dbkeeper-core/internal/config"
)

func TestNormalizeChannels_DingTalkWithSingleURL(t *testing.T) {
	cfg := config.Notify{
		Type: "http",
		Channels: []config.NotifyChannel{
			{
				Name:        "ding",
				Type:        "http",
				URL:         "https://oapi.dingtalk.com/robot/send",
				ChannelType: "dingtalk",
				AccessToken: "token-1",
				Sign:        "SECxxx",
				Keyword:     "dbkeeper",
			},
		},
	}

	channels := normalizeChannels(cfg)
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if channels[0].Method != http.MethodPost {
		t.Fatalf("expected POST method, got %s", channels[0].Method)
	}
	if channels[0].Keyword != "dbkeeper" {
		t.Fatalf("expected keyword dbkeeper, got %q", channels[0].Keyword)
	}
	if channels[0].URL != "https://oapi.dingtalk.com/robot/send" {
		t.Fatalf("unexpected url: %q", channels[0].URL)
	}
}

func TestNormalizeChannels_TopLevelURLCompatibility(t *testing.T) {
	cfg := config.Notify{
		Type:        "http",
		URL:         "https://oapi.dingtalk.com/robot/send",
		ChannelType: "dingtalk",
		AccessToken: "token-1",
		Sign:        "SECxxx",
		Keyword:     "dbkeeper",
	}

	channels := normalizeChannels(cfg)
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if channels[0].Method != http.MethodPost {
		t.Fatalf("expected POST method, got %s", channels[0].Method)
	}
	if channels[0].Keyword != "dbkeeper" {
		t.Fatalf("expected keyword dbkeeper, got %q", channels[0].Keyword)
	}
	if channels[0].URL != "https://oapi.dingtalk.com/robot/send" {
		t.Fatalf("unexpected url: %q", channels[0].URL)
	}
}

func TestNormalizeChannels_ChuckfangWithSingleURL(t *testing.T) {
	cfg := config.Notify{
		Type: "http",
		Channels: []config.NotifyChannel{
			{
				Name:        "cf",
				Type:        "http",
				URL:         "https://api.chuckfang.com/",
				ChannelType: "chuckfang",
				Method:      http.MethodPost,
			},
		},
	}

	channels := normalizeChannels(cfg)
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if channels[0].Method != http.MethodGet {
		t.Fatalf("expected forced GET method, got %s", channels[0].Method)
	}
	if channels[0].URL != "https://api.chuckfang.com/" {
		t.Fatalf("unexpected url: %q", channels[0].URL)
	}
}

func TestBuildDingTalkURL(t *testing.T) {
	now := time.Unix(0, 0)
	got, err := buildDingTalkURL("https://oapi.dingtalk.com/robot/send", "token-1", "SECabc", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result failed: %v", err)
	}
	q := parsed.Query()
	if q.Get("access_token") != "token-1" {
		t.Fatalf("expected access_token token-1, got %q", q.Get("access_token"))
	}
	if q.Get("timestamp") != "0" {
		t.Fatalf("expected timestamp 0, got %q", q.Get("timestamp"))
	}
	expectedSign := signDingTalk("0", "SECabc")
	if q.Get("sign") != expectedSign {
		t.Fatalf("unexpected sign: want=%q got=%q", expectedSign, q.Get("sign"))
	}
}

func TestBuildDingTalkBody(t *testing.T) {
	body, err := buildDingTalkBody(Payload{
		Message: "backup done",
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got["msgtype"] != "text" {
		t.Fatalf("expected msgtype text, got %#v", got["msgtype"])
	}
	textObj, ok := got["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text object")
	}
	if textObj["content"] != "backup done" {
		t.Fatalf("expected content backup done, got %#v", textObj["content"])
	}
}

func TestBuildDingTalkBody_WithKeyword(t *testing.T) {
	body, err := buildDingTalkBody(Payload{
		Message: "backup done",
	}, "dbkeeper")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	textObj, ok := got["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text object")
	}
	if textObj["content"] != "backup done\n关键字：dbkeeper" {
		t.Fatalf("expected prefixed content, got %#v", textObj["content"])
	}
}
