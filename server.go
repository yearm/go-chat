package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"strconv"
	"time"
)

//客户端
type Client struct {
	ID     string          `json:"id"`     //用户id
	Socket *websocket.Conn `json:"socket"` //连接的socket
	Send   chan []byte     `json:"send"`   //发送的消息
}

//客户端管理
type ClientManger struct {
	Clients    map[*Client]bool `json:"clients"`
	Broadcast  chan []byte      `json:"broadcast"`
	Register   chan *Client     `json:"register"`
	Unregister chan *Client     `json:"unregister"`
}

//消息
type Message struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Content   string `json:"content"`
}

var manager = ClientManger{
	Clients:    make(map[*Client]bool),
	Broadcast:  make(chan []byte),
	Register:   make(chan *Client),
	Unregister: make(chan *Client),
}

func (this *ClientManger) Start() {
	for {
		select {
		case conn := <-this.Register:
			this.Clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Sender: "system", Recipient: "all", Content: "A new socket has connected."})
			this.Send(jsonMessage, conn)

		case conn := <-this.Unregister:
			//如果是true就关闭连接
			if _, ok := this.Clients[conn]; ok {
				close(conn.Send)
				delete(this.Clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Sender: "system", Recipient: "all", Content: "A socket has disconnected."})
				manager.Send(jsonMessage, conn)
			}
		case message := <-this.Broadcast:
			//遍历已连接的客户端并发送消息
			for conn, _ := range this.Clients {
				select {
				case conn.Send <- message:
				default:
					close(conn.Send)
					delete(this.Clients, conn)
				}
			}
		}
	}
}

func (this *ClientManger) Send(message []byte, ignore *Client) {
	for conn, _ := range this.Clients {
		//不给屏蔽的连接发送消息
		if conn != ignore {
			conn.Send <- message
		}
	}
}

//接收从web端发过来的消息
func (this *Client) Read() {
	defer func() {
		manager.Unregister <- this
		this.Socket.Close()
	}()

	for {
		_, message, err := this.Socket.ReadMessage()
		if err != nil {
			manager.Unregister <- this
			this.Socket.Close()
			break
		}
		//如果没有错误信息就放入broadcast
		jsonMessage, _ := json.Marshal(&Message{Sender: this.ID, Recipient: "all", Content: string(message)})
		manager.Broadcast <- jsonMessage
	}
}

//将消息传入web端
func (this *Client) Write() {
	defer func() {
		this.Socket.Close()
	}()
	for {
		select {
		case message, ok := <-this.Send:
			//没有消息
			if !ok {
				this.Socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			//有消息就写入，发送给web端
			this.Socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}

func WsHandle(w http.ResponseWriter, r *http.Request) {
	//将http协议升级成websocket协议
	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return true
	}}).Upgrade(w, r, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	//每一次连接都会新开一个client
	client := &Client{ID: strconv.FormatInt(time.Now().Unix(), 10), Socket: conn, Send: make(chan []byte)}
	//注册一个新的连接
	manager.Register <- client
	go client.Read()
	go client.Write()
}

func main() {
	go manager.Start()
	http.HandleFunc("/ws", WsHandle)
	fmt.Println("Server is listening on :9999")
	http.ListenAndServe(":9999", nil)
}
