package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	clients   = make(map[*websocket.Conn]string)            // conn -> userID
	rooms     = make(map[string]map[string]*websocket.Conn) // roomID -> (userID -> conn)
	clientsMu sync.Mutex
	roomsMu   sync.Mutex
)

type Message struct {
	Type      string      `json:"type"` // "join", "offer", "answer", "candidate", etc.
	Room      string      `json:"room,omitempty"`
	From      string      `json:"from,omitempty"`
	To        string      `json:"to,omitempty"`
	SDP       interface{} `json:"sdp,omitempty"`
	ID        string      `json:"id,omitempty"`
	Candidate interface{} `json:"candidate,omitempty"`
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Erro ao fazer upgrade:", err)
		return
	}
	defer conn.Close()

	// Gera um ID único para este cliente
	userID := fmt.Sprintf("user-%p", conn)

	clientsMu.Lock()
	clients[conn] = userID
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()

		// Remove o usuário de todas as salas
		roomsMu.Lock()
		for roomID, users := range rooms {
			if _, ok := users[userID]; ok {
				delete(rooms[roomID], userID)
				// Notifica outros usuários que este usuário saiu
				for _, peerConn := range users {
					peerConn.WriteJSON(Message{
						Type: "peer-disconnected",
						ID:   userID,
					})
				}
			}
		}
		roomsMu.Unlock()
	}()

	// Informa ao cliente seu ID
	conn.WriteJSON(Message{
		Type: "init",
		ID:   userID,
	})

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			fmt.Println("Erro ao ler mensagem:", err)
			break
		}

		fmt.Printf("Mensagem recebida: %+v\n", msg)

		switch msg.Type {
		case "join":
			roomID := msg.Room
			if roomID == "" {
				conn.WriteJSON(Message{
					Type: "error",
					ID:   "room-required",
				})
				continue
			}

			roomsMu.Lock()
			// Cria a sala se não existir
			if _, ok := rooms[roomID]; !ok {
				rooms[roomID] = make(map[string]*websocket.Conn)
			}

			// Adiciona o usuário à sala
			rooms[roomID][userID] = conn

			// Notifica o usuário sobre os outros participantes na sala
			for peerID, peerConn := range rooms[roomID] {
				if peerID != userID {
					// Informa o novo usuário sobre os pares existentes
					conn.WriteJSON(Message{
						Type: "new-peer",
						ID:   peerID,
					})
					// Informa os pares existentes sobre o novo usuário
					peerConn.WriteJSON(Message{
						Type: "new-peer",
						ID:   userID,
					})
					// Registra os participantes no console do servidor
					fmt.Printf("Conectando usuários: %s e %s na sala %s\n", userID, peerID, roomID)
				}
			}
			roomsMu.Unlock()

		case "offer", "answer", "candidate":
			to := msg.To
			roomsMu.Lock()
			// Localiza o destinatário em qualquer sala
			var targetConn *websocket.Conn
			for _, users := range rooms {
				if conn, ok := users[to]; ok {
					targetConn = conn
					break
				}
			}
			roomsMu.Unlock()

			if targetConn != nil {
				// Adiciona o ID do remetente antes de encaminhar
				msg.From = userID
				targetConn.WriteJSON(msg)
			} else {
				fmt.Println("Destinatário não encontrado:", to)
			}
		}
	}
}

func main() {
	http.Handle("/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/ws", handleWS)

	fmt.Println("Servidor rodando em http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Erro ao iniciar servidor:", err)
	}
}
