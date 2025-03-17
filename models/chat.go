package models

//type struct of chat

//type Room With Chat

type RoomWithChat struct {
	Room  Room   `json:"room"`
	Chats []Chat `json:"chats"`
}

// type Room
type Room struct {
	RoomID    string `json:"room_id"`
	UserID    string `json:"user_id"`
	RoomTitle string `json:"room_title"`
	CreatedAt string `json:"created_at"`
}

type Chat struct {
	ChatID    string `json:"chat_id"`
	RoomID    string `json:"room_id"`
	UserID    string `json:"user_id"`
	Chat      string `json:"chat"`
	CreatedAt string `json:"created_at"`
}
