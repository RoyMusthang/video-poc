package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

var (
	clients   = make(map[*websocket.Conn]bool)
	broadcast = make(chan Message)
)

type Message struct {
	Type string `json:"type"` // "offer", "answer", "candidate"
	Data string `json:"data"`
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Erro ao fazer upgrade:", err)
		return
	}
	defer conn.Close()
	clients[conn] = true
	defer delete(clients, conn)

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			fmt.Println("Erro ao ler mensagem:", err)
			break
		}
		// Envia a mensagem para todos os outros clientes
		for client := range clients {
			if client != conn {
				client.WriteJSON(msg)
			}
		}
	}
}

func main() {
	http.Handle("/", http.FileServer(http.Dir("static")))
	http.HandleFunc("/ws", handleWS)

	fmt.Println("Servidor rodando em http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
