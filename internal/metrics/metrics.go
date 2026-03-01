package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RoomsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "poker_rooms_created_total",
		Help: "The total number of rooms created",
	})

	ActiveRooms = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "poker_active_rooms",
		Help: "The number of active rooms currently in the engine",
	})

	ActivePlayers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "poker_active_players_total",
		Help: "The total number of players currently in all rooms",
	})

	PlayersPerRoom = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "poker_players_per_room",
		Help:    "The distribution of the number of players in a room",
		Buckets: []float64{1, 2, 3, 5, 8, 13, 21, 34},
	})

	WSConnectionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "poker_ws_connections_active",
		Help: "The number of active WebSocket connections",
	})

	WSMessagesReceivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "poker_ws_messages_received_total",
		Help: "The total number of WebSocket messages received",
	}, []string{"action"})

	PlayerActionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "poker_player_actions_total",
		Help: "The total number of actions performed by players",
	}, []string{"action"})
)
