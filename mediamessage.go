package main

import tgbotapi "github.com/mymmrac/telego"

type MediaMessage struct {
	fileid            []string
	username          string
	fullname          string
	userid            int64
	caption           string
	entities          []tgbotapi.MessageEntity
	state             MessageState
	originalmessageid int
}

type MessageState int

const (
	MessageStateInit MessageState = iota
	MessageStateToBeSent
	MessageStateToDelete
)
