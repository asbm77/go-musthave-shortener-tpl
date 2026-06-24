package audit

import (
	"encoding/json"
	"time"
)

type Action string

const (
	ActionShorten Action = "shorten"
	ActionFollow  Action = "follow"
)

type Event struct {
	Timestamp int64  `json:"ts"`
	Action    Action `json:"action"`
	UserID    string `json:"user_id"`
	URL       string `json:"url"`
}

func NewEvent(action Action, userID, url string) *Event {
	return &Event{
		Timestamp: time.Now().Unix(),
		Action:    action,
		UserID:    userID,
		URL:       url,
	}
}

func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
