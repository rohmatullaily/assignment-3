package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var pool = &Pool{}
var messages = []Message{}

func main() {
	pool = NewPool()
	go pool.Start()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("index.html")
		if err != nil {
			w.Write([]byte("error : " + err.Error()))
			return
		}

		tmpl.Execute(w, nil)
	})
	http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("chat.html")
		if err != nil {
			w.Write([]byte("error : " + err.Error()))
			return
		}

		tmpl.Execute(w, nil)
	})
	http.HandleFunc("/chat/ws", wsHandler)

	log.Println("server running at port :8080")
	http.ListenAndServe(":8080", nil)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("error when try to upgrade connection :", err.Error())
		return
	}

	defer conn.Close()

	client := Client{
		Conn: conn,
		ID:   name,
		Pool: pool,
	}

	pool.Register <- &client
	client.Read()
}

func Reader(conn *websocket.Conn) {
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("error when try to read message :", err.Error())
			return
		}
		log.Println("message type :", msgType)
		log.Println("received message :", string(msg))

		err = conn.WriteMessage(msgType, []byte("receive : "+string(msg)))
		if err != nil {
			log.Println("error when try to write message :", err.Error())
			return
		}
	}
}

func Writer(conn *websocket.Conn) {
	for {
		log.Println("sending ...")
		msgType, msg, err := conn.NextReader()
		if err != nil {
			log.Println("error when try to Next Reader :", err.Error())
			return
		}

		w, err := conn.NextWriter(msgType)
		if err != nil {
			log.Println("error when try to Next Writer :", err.Error())
			return
		}

		_, err = io.Copy(w, msg)
		if err != nil {
			log.Println("error when try to Copy Message :", err.Error())
			return
		}

		defer w.Close()
	}
}

type Client struct {
	ID   string
	Conn *websocket.Conn
	Pool *Pool
}

func (c *Client) Read() {
	defer func() {
		c.Pool.Unregister <- c
		c.Conn.Close()
	}()

	for {
		msgType, msg, err := c.Conn.ReadMessage()
		if err != nil {
			log.Println("error when try to read message :", err.Error())
			return
		}

		message := Message{}

		err = json.Unmarshal(msg, &message)
		if err != nil {
			log.Println("error when try to parsing message :", err.Error())
			return
		}
		message.Type = msgType
		messages = append(messages, message)
		c.ID = message.Name
		c.Pool.Broadcast <- message
		log.Println("message type :", msgType)
		log.Println("received message :", string(msg))
	}

}

type Message struct {
	Type int    `json:"type"`
	Body string `json:"message"`
	Name string `json:"name"`
}

type Pool struct {
	Register   chan *Client
	Unregister chan *Client
	Clients    map[*Client]bool
	Broadcast  chan Message
}

func NewPool() *Pool {
	return &Pool{
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan Message),
	}
}

func (p *Pool) Start() {
	for {
		select {
		case client := <-p.Register:
			p.Clients[client] = true
			fmt.Println("Size of Connection Pool: ", len(p.Clients))

			// Broadcast a message to all connected clients when a new user joins
			msg := Message{
				Type: 0,
				Body: client.ID + " Joined the chat...",
			}
			messages = append(messages, msg)
			log.Println("new user joined")
			for _, m := range messages {
				client.Conn.WriteJSON(m)
			}

		case client := <-p.Unregister:
			delete(p.Clients, client)
			msg := Message{
				Type: 0,
				Body: client.ID + " disconnected ...",
			}
			log.Println("user", client, "logout")
			fmt.Println("Size of Connection Pool: ", len(p.Clients))
			for c := range p.Clients {
				c.Conn.WriteJSON(msg)
			}

		case message := <-p.Broadcast:
			fmt.Println("Sending message to all clients in Pool")
			for c := range p.Clients {
				c.Conn.WriteJSON(message)
			}
		}
	}
}
