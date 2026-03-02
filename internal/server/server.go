package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"planning-poker-go/internal/engine"
	"planning-poker-go/internal/metrics"
	"planning-poker-go/internal/models"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // For demo purposes
	},
}

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	RoomId   uuid.UUID
	PlayerId string
}

type Hub struct {
	Rooms      map[uuid.UUID]map[*Client]bool
	Broadcast  chan HubEvent
	Register   chan *Client
	Unregister chan *Client
	Mu         sync.RWMutex
}

type HubEvent struct {
	RoomId  uuid.UUID
	Message models.HubMessage
}

func NewHub() *Hub {
	return &Hub{
		Rooms:      make(map[uuid.UUID]map[*Client]bool),
		Broadcast:  make(chan HubEvent),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mu.Lock()
			if h.Rooms[client.RoomId] == nil {
				h.Rooms[client.RoomId] = make(map[*Client]bool)
			}
			h.Rooms[client.RoomId][client] = true
			h.Mu.Unlock()
			metrics.WSConnectionsActive.Inc()
		case client := <-h.Unregister:
			h.Mu.Lock()
			if _, ok := h.Rooms[client.RoomId]; ok {
				delete(h.Rooms[client.RoomId], client)
				close(client.Send)
				if len(h.Rooms[client.RoomId]) == 0 {
					delete(h.Rooms, client.RoomId)
				}
			}
			h.Mu.Unlock()
			metrics.WSConnectionsActive.Dec()
		case event := <-h.Broadcast:
			h.Mu.RLock()
			msg, _ := json.Marshal(event.Message)
			for client := range h.Rooms[event.RoomId] {
				select {
				case client.Send <- msg:
				default:
					close(client.Send)
					delete(h.Rooms[event.RoomId], client)
				}
			}
			h.Mu.RUnlock()
		}
	}
}

type Server struct {
	Engine *engine.Engine
	Hub    *Hub
}

func (s *Server) HandleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CardSet string `json:"cardSet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("Failed to decode create room request", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := s.Engine.CreateRoom(req.CardSet)
	if err != nil {
		slog.Error("Failed to create room", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	roomIdStr := r.URL.Query().Get("roomId")
	roomId, err := uuid.Parse(roomIdStr)
	if err != nil {
		slog.Warn("Invalid room ID in WebSocket request", "roomId", roomIdStr)
		http.Error(w, "invalid room id", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade connection to WebSocket", "error", err, "roomId", roomId)
		return
	}

	slog.Info("WebSocket connection established", "roomId", roomId, "remoteAddr", r.RemoteAddr)

	client := &Client{Hub: s.Hub, Conn: conn, Send: make(chan []byte, 256), RoomId: roomId}
	s.Hub.Register <- client

	go client.writePump()
	go client.readPump(s)
}

func (c *Client) readPump(s *Server) {
	defer func() {
		if c.PlayerId != "" {
			if name, ok := s.Engine.DisconnectPlayer(c.RoomId, c.PlayerId); ok {
				slog.Info("Player disconnected", "roomId", c.RoomId, "playerName", name)
				s.broadcastUpdate(c.RoomId)
			}
		}
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("WebSocket read error", "error", err, "roomId", c.RoomId)
			}
			break
		}

		var req struct {
			Action  string          `json:"action"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(message, &req); err != nil {
			slog.Warn("Failed to unmarshal WS message", "error", err, "roomId", c.RoomId)
			continue
		}

		metrics.WSMessagesReceivedTotal.WithLabelValues(req.Action).Inc()
		s.handleAction(c, req.Action, req.Payload)
	}
}

func (c *Client) writePump() {
	defer c.Conn.Close()
	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Warn("WebSocket write error", "error", err, "roomId", c.RoomId)
				return
			}
		}
	}
}

func (s *Server) handleAction(c *Client, action string, payload json.RawMessage) {
	playerName := s.getPlayerName(c)

	// If player is not recognized and trying to do something other than join, ignore or close
	if playerName == "Unknown" && action != "join" {
		return
	}

	switch action {
	case "join":
		var p struct {
			Name       string    `json:"name"`
			RecoveryId uuid.UUID `json:"recoveryId"`
			Type       string    `json:"type"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			slog.Warn("Join unmarshal error", "error", err, "roomId", c.RoomId)
			return
		}
		player, err := s.Engine.JoinRoom(c.RoomId, p.RecoveryId, p.Name, c.Conn.RemoteAddr().String(), models.PlayerType(p.Type))
		if err != nil || player == nil {
			slog.Error("JoinRoom error", "error", err, "playerIsNil", player == nil, "roomId", c.RoomId)
			return
		}
		c.PlayerId = player.Id
		
		// Send success to client
		successMsg, _ := json.Marshal(models.HubMessage{
			Type:    models.MessageTypeJoinSuccess,
			Payload: player,
		})
		c.Send <- successMsg

		s.broadcastUpdate(c.RoomId)
		s.broadcastLog(c.RoomId, player.Name, "Joined the room")

	case "vote":
		var p struct {
			Vote string `json:"vote"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return
		}
		if err := s.Engine.Vote(c.RoomId, c.PlayerId, p.Vote); err != nil {
			slog.Warn("Vote error", "playerName", playerName, "error", err, "roomId", c.RoomId)
			return
		}
		s.broadcastLog(c.RoomId, playerName, "Voted")
		s.broadcastUpdate(c.RoomId)

	case "unvote":
		s.Engine.UnVote(c.RoomId, c.PlayerId)
		s.broadcastLog(c.RoomId, playerName, "Redacted their vote")
		s.broadcastUpdate(c.RoomId)

	case "show":
		s.Engine.ShowVotes(c.RoomId)
		s.broadcastLog(c.RoomId, playerName, "Made all votes visible")
		s.broadcastUpdate(c.RoomId)

	case "clear":
		s.Engine.ClearVotes(c.RoomId)
		s.broadcastLog(c.RoomId, playerName, "Cleared all votes")
		s.broadcastUpdate(c.RoomId)
		s.Hub.Broadcast <- HubEvent{RoomId: c.RoomId, Message: models.HubMessage{Type: models.MessageTypeClear}}

	case "kick":
		var p struct {
			PublicId int `json:"publicId"`
		}
		json.Unmarshal(payload, &p)
		kickedPrivateId, err := s.Engine.KickPlayer(c.RoomId, p.PublicId)
		if err == nil {
			s.kickClient(c.RoomId, kickedPrivateId)
			s.broadcastUpdate(c.RoomId)
		}

	case "changeType":
		var p struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return
		}
		
		server, ok := s.Engine.GetServer(c.RoomId)
		if !ok { return }
		player, ok := server.Players[c.PlayerId]
		if !ok { return }

		player.Type = models.PlayerType(p.Type)
		// Clear vote if they become an observer
		if player.Type == models.Observer {
			s.Engine.UnVote(c.RoomId, c.PlayerId)
		}

		s.broadcastLog(c.RoomId, playerName, "Changed their player type to "+p.Type)
		s.broadcastUpdate(c.RoomId)

	case "chat":
		var p struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return
		}
		s.broadcastChat(c.RoomId, playerName, p.Message)
	case "leave":
		if c.PlayerId != "" {
			if name, ok := s.Engine.LeaveRoom(c.RoomId, c.PlayerId); ok {
				s.broadcastUpdate(c.RoomId)
				s.broadcastLog(c.RoomId, name, "Left the room")
				c.PlayerId = "" // Prevent readPump from marking as disconnected
			}
		}
	}
}

func (s *Server) getPlayerName(c *Client) string {
	if c.PlayerId == "" {
		return "Unknown"
	}
	server, ok := s.Engine.GetServer(c.RoomId)
	if !ok {
		return "Unknown"
	}
	player, ok := server.Players[c.PlayerId]
	if !ok {
		return "Unknown"
	}
	return player.Name
}

func (s *Server) broadcastChat(roomId uuid.UUID, user, message string) {
	s.Hub.Broadcast <- HubEvent{
		RoomId: roomId,
		Message: models.HubMessage{
			Type: models.MessageTypeChat,
			Payload: models.ChatMessage{
				User:      user,
				Message:   message,
				Timestamp: time.Now(),
			},
		},
	}
}

func (s *Server) broadcastUpdate(roomId uuid.UUID) {
	server, _ := s.Engine.GetServer(roomId)
	s.Hub.Broadcast <- HubEvent{
		RoomId: roomId,
		Message: models.HubMessage{
			Type:    models.MessageTypeUpdated,
			Payload: server,
		},
	}
}

func (s *Server) broadcastLog(roomId uuid.UUID, user, message string) {
	s.Hub.Broadcast <- HubEvent{
		RoomId: roomId,
		Message: models.HubMessage{
			Type: models.MessageTypeLog,
			Payload: models.LogMessage{
				User:      user,
				Message:   message,
				Timestamp: time.Now(),
			},
		},
	}
}

func (s *Server) kickClient(roomId uuid.UUID, playerId string) {
	s.Hub.Mu.RLock()
	defer s.Hub.Mu.RUnlock()

	for client := range s.Hub.Rooms[roomId] {
		if client.PlayerId == playerId {
			// Send kicked message
			msg, _ := json.Marshal(models.HubMessage{
				Type: models.MessageTypeKicked,
			})
			select {
			case client.Send <- msg:
			default:
			}
		}
	}
}
