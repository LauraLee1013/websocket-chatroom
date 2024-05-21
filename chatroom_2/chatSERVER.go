package main

import (
	"encoding/binary"
	"hash/crc32"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 訊息
type Message struct {
	I           byte
	SenderID    int32
	RecipientID int32
	Timestamp   uint32
	MessageLen  uint32
	Message     string
	Checksum    uint16
	F           byte
}

var (
	clients   = make(map[*websocket.Conn]int32)
	clientsMu sync.Mutex
	client1   *websocket.Conn
)

func userID() int32 {
	rand.Seed(time.Now().UnixNano())
	userid := rand.Int31()
	return userid
}

// 1.處理websocket連線要求
// 2.將client的ID發送給client
// 3.將ID分別發給第一個用戶和第二個用戶
// 4.讀取client訊息
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 建立WebSocket連線
	conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}
	defer conn.Close()
	clientsMu.Lock()
	clientID := userID()
	clients[conn] = clientID
	log.Printf("%d Client connected to server", clientID)

	// 將client的ID發送給client
	sendIDToClient := Message{
		I:           0x2F,
		SenderID:    0,
		RecipientID: clients[conn],
		Timestamp:   uint32(time.Now().Unix()),
		Message:     strconv.Itoa(int(clients[conn])),
		MessageLen:  uint32(len(strconv.Itoa(int(clients[conn])))),
		Checksum:    0,
		F:           0x2F,
	}
	if err := sendMessage(conn, sendIDToClient); err != nil {
		log.Printf("Error sending ID to client %d: %v", clientID, err)
	}
	if len(clients) == 1 {
		client1 = conn
		log.Printf("The first client is: %d", clients[client1])
	}

	if len(clients) == 2 {
		log.Printf("The second client is: %d", clients[conn])

		sendFirstIDToSecond := Message{
			I:           0x2F,
			SenderID:    0,
			RecipientID: clients[conn],
			Timestamp:   uint32(time.Now().Unix()),
			Message:     strconv.Itoa(int(clients[client1])),
			MessageLen:  uint32(len(strconv.Itoa(int(clients[client1])))),
			Checksum:    0,
			F:           0x2F,
		}
		if err := sendMessage(conn, sendFirstIDToSecond); err != nil {
			log.Printf("Error sending first ID to second client: %v", err)
		}

		sendSecondIDToFirst := Message{
			I:           0x2F,
			SenderID:    0,
			RecipientID: clients[client1],
			Timestamp:   uint32(time.Now().Unix()),
			Message:     strconv.Itoa(int(clientID)),
			MessageLen:  uint32(len(strconv.Itoa(int(clientID)))),
			Checksum:    0,
			F:           0x2F,
		}
		if err := sendMessage(client1, sendSecondIDToFirst); err != nil {
			log.Printf("Error sending second ID to first client: %v", err)
		}
	}
	clientsMu.Unlock()

	for {
		// 讀client的訊息
		_, mesg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error receiving message from client %d: %v", clientID, err)
			break
		}

		msg := Message{

			I:           mesg[0],
			SenderID:    int32(binary.LittleEndian.Uint32(mesg[1:5])),
			RecipientID: int32(binary.LittleEndian.Uint32(mesg[5:9])),
			Timestamp:   binary.LittleEndian.Uint32(mesg[9:13]),
			MessageLen:  binary.LittleEndian.Uint32(mesg[13:17]),
			Message:     string(mesg[17 : 17+mesg[13]]),
			Checksum:    binary.LittleEndian.Uint16(mesg[17+mesg[13] : 19+mesg[13]]),
			F:           mesg[19+mesg[13]],
		}

		log.Printf("Received message from %d: %+v", clientID, msg)

		broadcast(msg)
	}

	clientsMu.Lock()
	delete(clients, conn)
	clientsMu.Unlock()

	log.Printf("%d Client disconnected from server", clientID)
}

// 用sendmessage發送recipientID給client
func broadcast(msg Message) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for conn := range clients {
		if clients[conn] == msg.RecipientID {
			if err := sendMessage(conn, msg); err != nil {
				log.Printf("Error broadcasting message to client %d: %v", clients[conn], err)
			}
		}
	}
}

func sendMessage(conn *websocket.Conn, msg Message) error {
	buf := serializeMessage(msg)
	return conn.WriteMessage(websocket.BinaryMessage, buf)
}

// 解析訊息
func serializeMessage(msg Message) []byte {
	messageLen := uint32(len(msg.Message))
	buf := make([]byte, 23+messageLen)
	buf[0] = msg.I
	binary.LittleEndian.PutUint32(buf[1:], uint32(msg.SenderID))
	binary.LittleEndian.PutUint32(buf[5:], uint32(msg.RecipientID))
	binary.LittleEndian.PutUint32(buf[9:], msg.Timestamp)
	binary.LittleEndian.PutUint32(buf[13:], messageLen)
	copy(buf[17:], []byte(msg.Message))
	Checksum := crc32.ChecksumIEEE(buf[:17+messageLen])
	binary.LittleEndian.PutUint16(buf[17+messageLen:], uint16(Checksum))
	buf[19+messageLen] = msg.F
	return buf
}

// 監聽到8080
func main() {
	http.HandleFunc("/ws", handleWebSocket)
	log.Println("Server listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
