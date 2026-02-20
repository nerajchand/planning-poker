package models

import (
	"time"

	"github.com/google/uuid"
)

type PlayerType string

const (
	Participant PlayerType = "Participant"
	Observer    PlayerType = "Observer"
)

type PlayerMode string

const (
	Awake  PlayerMode = "Awake"
	Asleep PlayerMode = "Asleep"
)

type Player struct {
	Id        string     `json:"id,omitempty"` // Private ID
	PublicId  int        `json:"publicId"`
	RecoveryId uuid.UUID  `json:"recoveryId"`
	Name      string     `json:"name"`
	Type      PlayerType `json:"type"`
	Mode      PlayerMode `json:"mode"`
}

type PokerSession struct {
	CardSet []string          `json:"cardSet"`
	Votes   map[string]string `json:"votes"` // Key is PublicId as string
	IsShown bool              `json:"isShown"`
}

type PokerServer struct {
	Id             uuid.UUID          `json:"id"`
	Players        map[string]*Player `json:"players"` // Key is Private ID
	CurrentSession *PokerSession      `json:"currentSession"`
	LastAccess     time.Time          `json:"-"`
}

// Hub Messages
type MessageType string

const (
	MessageTypeUpdated MessageType = "updated"
	MessageTypeKicked  MessageType = "kicked"
	MessageTypeLog     MessageType = "log"
	MessageTypeClear   MessageType = "clear"
	MessageTypeJoinSuccess MessageType = "join_success"
	MessageTypeChat    MessageType = "chat"
)

type HubMessage struct {
	Type    MessageType `json:"type"`
	Payload interface{} `json:"payload"`
}

type LogMessage struct {
	User      string    `json:"user"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type ChatMessage struct {
	User      string    `json:"user"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}
