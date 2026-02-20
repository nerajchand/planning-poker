package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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
		return nil, errors.New("room not found")
	}

	// Check if player is recovering
	for _, p := range server.Players {
		if p.RecoveryId == recoveryId {
			// Update existing player
			delete(server.Players, p.Id) // Remove old mapping if private ID changed
			p.Id = privateId
			p.Mode = models.Awake
			p.Name = playerName
			p.Type = pType
			server.Players[privateId] = p
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

	server.CurrentSession.Votes[fmt.Sprintf("%d", player.PublicId)] = vote
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

	delete(server.CurrentSession.Votes, fmt.Sprintf("%d", player.PublicId))
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
			return id, nil
		}
	}

	return "", errors.New("player not found")
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

	return name, true
}

func (e *Engine) CleanupOldRooms(maxAge time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for id, s := range e.servers {
		if now.Sub(s.LastAccess) > maxAge {
			delete(e.servers, id)
		}
	}
}
