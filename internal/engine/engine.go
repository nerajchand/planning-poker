package engine

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"planning-poker-go/internal/metrics"
	"planning-poker-go/internal/models"

	"github.com/google/uuid"
)

type Engine struct {
	servers map[uuid.UUID]*models.PokerServer
	mu      sync.RWMutex
}

func NewEngine() *Engine {
	return &Engine{
		servers: make(map[uuid.UUID]*models.PokerServer),
	}
}

func (e *Engine) CreateRoom(desiredCardSet string) (uuid.UUID, error) {
	cards := strings.Split(desiredCardSet, ",")
	var cleanedCards []string
	for _, c := range cards {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			cleanedCards = append(cleanedCards, trimmed)
		}
	}

	if len(cleanedCards) == 0 {
		slog.Warn("Attempted to create room with empty card set")
		return uuid.Nil, errors.New("card set cannot be empty")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	id := uuid.New()
	e.servers[id] = &models.PokerServer{
		Id:      id,
		Players: make(map[string]*models.Player),
		CurrentSession: &models.PokerSession{
			CardSet: cleanedCards,
			Votes:   make(map[string]string),
		},
		LastAccess: time.Now(),
	}

	metrics.RoomsCreatedTotal.Inc()
	metrics.ActiveRooms.Set(float64(len(e.servers)))
	slog.Info("Room created", "roomId", id, "cardSet", desiredCardSet)

	return id, nil
}

func (e *Engine) GetServer(id uuid.UUID) (*models.PokerServer, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.servers[id]
	if ok {
		s.LastAccess = time.Now()
	}
	return s, ok
}

func (e *Engine) JoinRoom(id uuid.UUID, recoveryId uuid.UUID, playerName string, privateId string, pType models.PlayerType) (*models.Player, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[id]
	if !ok {
		slog.Warn("Player tried to join non-existent room", "roomId", id)
		return nil, errors.New("room not found")
	}

	// Check if player is recovering
	for _, p := range server.Players {
		if p.RecoveryId == recoveryId {
			// Update existing player
			delete(server.Players, p.Id) // Remove old mapping if private ID changed
			p.Id = privateId
			p.Mode = models.Awake
			// Only update name/type if they were provided and not empty
			if playerName != "" {
				p.Name = playerName
			}
			if pType != "" {
				p.Type = pType
			}
			server.Players[privateId] = p
			slog.Info("Player recovered session", "roomId", id, "playerName", p.Name, "type", p.Type)
			return p, nil
		}
	}

	// New player
	publicId := 1
	if len(server.Players) > 0 {
		var ids []int
		for _, p := range server.Players {
			ids = append(ids, p.PublicId)
		}
		sort.Ints(ids)
		publicId = ids[len(ids)-1] + 1
	}

	player := &models.Player{
		Id:         privateId,
		PublicId:   publicId,
		RecoveryId: recoveryId,
		Name:       playerName,
		Type:       pType,
		Mode:       models.Awake,
	}

	server.Players[privateId] = player
	
	metrics.ActivePlayers.Inc()
	metrics.PlayersPerRoom.Observe(float64(len(server.Players)))
	slog.Info("Player joined room", "roomId", id, "playerName", playerName, "type", pType, "totalPlayers", len(server.Players))
	
	return player, nil
}

func (e *Engine) Vote(serverId uuid.UUID, privateId string, vote string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return errors.New("room not found")
	}

	player, ok := server.Players[privateId]
	if !ok {
		return errors.New("player not found")
	}

	if player.Type == models.Observer {
		return errors.New("observers cannot vote")
	}

	if server.CurrentSession.IsShown {
		return errors.New("cannot change vote once revealed")
	}

	player.Mode = models.Awake // If they vote, they are awake
	server.CurrentSession.Votes[fmt.Sprintf("%d", player.PublicId)] = vote
	
	metrics.PlayerActionsTotal.WithLabelValues("vote").Inc()
	
	return nil
}

func (e *Engine) UnVote(serverId uuid.UUID, privateId string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return errors.New("room not found")
	}

	if server.CurrentSession.IsShown {
		return errors.New("cannot redact vote once revealed")
	}

	player, ok := server.Players[privateId]
	if !ok {
		return errors.New("player not found")
	}

	player.Mode = models.Awake
	delete(server.CurrentSession.Votes, fmt.Sprintf("%d", player.PublicId))
	
	metrics.PlayerActionsTotal.WithLabelValues("unvote").Inc()
	
	return nil
}

func (e *Engine) ClearVotes(serverId uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return errors.New("room not found")
	}

	server.CurrentSession.Votes = make(map[string]string)
	server.CurrentSession.IsShown = false
	
	metrics.PlayerActionsTotal.WithLabelValues("clear").Inc()
	
	return nil
}

func (e *Engine) ShowVotes(serverId uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return errors.New("room not found")
	}

	server.CurrentSession.IsShown = true
	
	metrics.PlayerActionsTotal.WithLabelValues("show").Inc()
	
	return nil
}

func (e *Engine) KickPlayer(serverId uuid.UUID, kickedPublicId int) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return "", errors.New("room not found")
	}

	for id, p := range server.Players {
		if p.PublicId == kickedPublicId {
			delete(server.Players, id)
			delete(server.CurrentSession.Votes, fmt.Sprintf("%d", p.PublicId))
			
			metrics.ActivePlayers.Dec()
			slog.Info("Player kicked", "roomId", serverId, "publicId", kickedPublicId, "playerName", p.Name)
			
			return id, nil
		}
	}

	return "", errors.New("player not found")
}

func (e *Engine) DisconnectPlayer(serverId uuid.UUID, privateId string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return "", false
	}

	player, ok := server.Players[privateId]
	if !ok {
		return "", false
	}

	player.Mode = models.Asleep
	slog.Info("Player marked asleep", "roomId", serverId, "playerName", player.Name)
	return player.Name, true
}

func (e *Engine) LeaveRoom(serverId uuid.UUID, privateId string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	server, ok := e.servers[serverId]
	if !ok {
		return "", false
	}

	player, ok := server.Players[privateId]
	if !ok {
		return "", false
	}

	name := player.Name
	delete(server.Players, privateId)
	delete(server.CurrentSession.Votes, fmt.Sprintf("%d", player.PublicId))
	
	metrics.ActivePlayers.Dec()
	slog.Info("Player left room", "roomId", serverId, "playerName", name)

	return name, true
}

func (e *Engine) CleanupOldRooms(maxAge time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	cleaned := 0
	playersRemoved := 0
	for id, s := range e.servers {
		if now.Sub(s.LastAccess) > maxAge {
			playersRemoved += len(s.Players)
			delete(e.servers, id)
			cleaned++
		}
	}
	
	if cleaned > 0 {
		metrics.ActiveRooms.Set(float64(len(e.servers)))
		metrics.ActivePlayers.Sub(float64(playersRemoved))
		slog.Info("Cleaned up old rooms", "roomsRemoved", cleaned, "playersRemoved", playersRemoved, "activeRooms", len(e.servers))
	}
}
