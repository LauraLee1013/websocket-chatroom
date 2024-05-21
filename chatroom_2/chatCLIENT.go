package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

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

var recipientID int32

func main() {
	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatalf("Error connecting to server: %v", err)
	}
	defer ws.Close()
	log.Println("Connected to server")

	var userID int32
	_, msgBytes, err := ws.ReadMessage()
	if err != nil {
		log.Fatalf("Error receiving message from server: %v", err)
	}

	senderid, err := strconv.ParseInt(string(msgBytes[17:17+msgBytes[13]]), 10, 32)
	if err != nil {
		log.Printf("Error converting recipient ID to int: %v", err)
	}
	userID = int32(senderid)
	fmt.Println("user ID is:", userID)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go ReceiveMessage(ws, interrupt)

	SendMessages(ws, interrupt, userID)
}

func ReceiveMessage(ws *websocket.Conn, interrupt <-chan os.Signal) {
	for {
		select {
		case <-interrupt:
			fmt.Println("\nReceived interrupt signal. Closing connection...")
			return
		default:
			_, msgBytes, err := ws.ReadMessage()
			if err != nil {
				log.Fatalf("Error receiving message from server: %v", err)
			}

			msg, err := DeserializeMessage(msgBytes)
			if err != nil {
				log.Printf("Error deserializing message: %v", err)
				continue
			}

			if msg.SenderID == 0 {
				recipientIDInt64, err := strconv.ParseInt(msg.Message, 10, 32)
				if err != nil {
					log.Printf("Error converting recipient ID to int: %v", err)
					continue
				}
				recipientID = int32(recipientIDInt64)
				fmt.Printf("Another client is: %d\n", recipientID)
				fmt.Println("enter message:")
			} else {
				fmt.Printf("Received message from client %d: %s\n", msg.SenderID, msg.Message)
				fmt.Println("enter message:")
			}
		}
	}
}

func SendMessages(ws *websocket.Conn, interrupt <-chan os.Signal, senderID int32) {
	for {
		select {
		case <-interrupt:
			fmt.Println("\nReceived interrupt signal. Closing connection...")
			return
		default:
			var message string
			inputReader := bufio.NewReader(os.Stdin)
			message, err := inputReader.ReadString('\n')
			if err != nil {
				log.Printf("Error reading input: %v", err)
				continue
			}

			message = strings.TrimRight(message, "\n")

			timestamp := uint32(time.Now().Unix())

			msg := Message{
				I:           0x2F,
				SenderID:    senderID,
				RecipientID: recipientID,
				Timestamp:   timestamp,
				Message:     message,
				MessageLen:  uint32(len(message)),
				Checksum:    0,
				F:           0x2F,
			}

			msgBytes := SerializeMessage(msg)

			if err := ws.WriteMessage(websocket.BinaryMessage, msgBytes); err != nil {
				log.Fatalf("Error sending message to server: %v", err)
			}
		}
	}
}

func SerializeMessage(msg Message) []byte {
	messageLen := uint32(len(msg.Message))
	buf := make([]byte, 23+messageLen)
	buf[0] = msg.I
	binary.LittleEndian.PutUint32(buf[1:], uint32(msg.SenderID))
	binary.LittleEndian.PutUint32(buf[5:], uint32(msg.RecipientID))
	binary.LittleEndian.PutUint32(buf[9:], msg.Timestamp)
	binary.LittleEndian.PutUint32(buf[13:], messageLen) // 使用正确的 messageLen
	copy(buf[17:], []byte(msg.Message))
	Checksum := crc32.ChecksumIEEE(buf[:17+messageLen])
	binary.LittleEndian.PutUint16(buf[17+messageLen:], uint16(Checksum))
	buf[19+messageLen] = msg.F
	return buf
}

func DeserializeMessage(data []byte) (Message, error) {
	msg := Message{
		I:           data[0],
		SenderID:    int32(binary.LittleEndian.Uint32(data[1:5])),
		RecipientID: int32(binary.LittleEndian.Uint32(data[5:9])),
		Timestamp:   binary.LittleEndian.Uint32(data[9:13]),
		MessageLen:  binary.LittleEndian.Uint32(data[13:17]),
		Message:     string(data[17 : 17+data[13]]),
		Checksum:    binary.LittleEndian.Uint16(data[17+data[13] : 19+data[13]]),
		F:           data[19+data[13]],
	}
	return msg, nil
}
