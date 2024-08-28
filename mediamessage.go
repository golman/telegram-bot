package main

type MediaMessage struct {
	fileid   []string
	username string
	fullname string
	userid   int64
	caption  string
	state    MessageState
}

type MessageState int

const (
	MessageStateInit MessageState = iota
	MessageStateToBeSent
	MessageStateToDelete
)
